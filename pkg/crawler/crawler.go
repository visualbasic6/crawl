package crawler

import (
    "context"
    "encoding/hex"
    "time"

    "github.com/migalabs/armiarma/src/utils"
    "github.com/migalabs/eth-light-crawler/pkg/config"
    "github.com/migalabs/eth-light-crawler/pkg/db"
    "github.com/migalabs/eth-light-crawler/pkg/discv5"
    ut "github.com/migalabs/eth-light-crawler/pkg/utils"

    "github.com/pkg/errors"

    "github.com/ethereum/go-ethereum/p2p/enode"
    "github.com/protolambda/zrnt/eth2/beacon/common"
    log "github.com/sirupsen/logrus"
)

var (
    EmptyBytes error = errors.New("array of bytes is empty")
)

type Crawler struct {
    ctx context.Context

    startT   time.Time
    duration time.Duration

    ethNode       *enode.LocalNode
    discv5Service *discv5.Discv5Service
    dbClient      *db.DBClient

    enrCache map[enode.ID]uint64
}

func New(
    ctx context.Context,
    dbEndpoint string,
    dbPath string,
    port int,
    resetDB bool,
) (*Crawler, error) {
    // Generate a new PrivKey
    privK, err := ut.GenNewPrivKey()
    if err != nil {
        return nil, errors.Wrap(err, "error generating privkey")
    }

    // Init the Ethereum peerstore
    enodeDB, err := enode.OpenDB(dbPath)
    if err != nil {
        return nil, err
    }

    // open the Postgres DB client
    sqlDB, err := db.NewDBClient(ctx, dbEndpoint, true, resetDB)
    if err != nil {
        return nil, err
    }

    // generate the local enode
    ethNode := enode.NewLocalNode(enodeDB, privK)

    // track known nodeIDs
    enrCache := make(map[enode.ID]uint64)

    // define how we handle newly discovered ENRs
    enrHandler := func(node *enode.Node) {
        err := node.ValidateComplete()
        if err != nil {
            log.Warnf("error validating the ENR: %s", err.Error())
        }
        // basic fields
        id := node.ID()
        seq := node.Seq()
        ip := node.IP()
        udp := node.UDP()
        tcp := node.TCP()
        pubkey := node.Pubkey()

        // ----------------------------------------------------------
        // ### CHANGE 1 ###  Safely parse eth2 data
        // ----------------------------------------------------------
        eth2Data, ok, parseErr := utils.ParseNodeEth2Data(*node)
        if parseErr != nil {
            // If parse fails, you can skip the node or just set a blank struct:
            log.Warnf("eth2 data parsing error on %s: %v", id, parseErr)
            eth2Data = new(common.Eth2Data) // blank fallback
        }
        if !ok {
            // no eth2 data found
            eth2Data = new(common.Eth2Data)
        }

        // ----------------------------------------------------------
        // ### CHANGE 2 ###  Safely parse attnets
        // ----------------------------------------------------------
        attnets, ok, attErr := discv5.ParseAttnets(*node)
        if attErr != nil {
            log.Warnf("attnets parsing error on %s: %v", id, attErr)
            attnets = new(discv5.Attnets)
        }
        if !ok {
            attnets = new(discv5.Attnets)
        }

        // build the local struct
        enrNode := discv5.NewEnrNode(id)
        enrNode.Seq = seq
        enrNode.IP = ip
        enrNode.TCP = tcp
        enrNode.UDP = udp
        enrNode.Pubkey = pubkey
        enrNode.Eth2Data = eth2Data
        enrNode.Attnets = attnets

        // log it
        log.WithFields(log.Fields{
            "node_id":     id,
            "ip":          ip,
            "udp":         udp,
            "tcp":         tcp,
            "fork_digest": eth2Data.ForkDigest,
            "fork_epoch":  eth2Data.NextForkEpoch,
            "attnets":     hex.EncodeToString(attnets.Raw[:]),
            "att_number":  attnets.NetNumber,
            "enr":         node.String(),
        }).Info("Eth node found")

        // decide whether to insert or update
        prevSeq, known := enrCache[enrNode.ID]
        if !known {
            sqlDB.InsertIntoDB(enrNode)
        } else if enrNode.Seq > prevSeq {
            sqlDB.UpdateInDB(enrNode)
        }
        enrCache[enrNode.ID] = enrNode.Seq
    }

    // spin up discv5
    discv5Serv, err := discv5.NewService(ctx, port, privK, ethNode, config.EthBootonodes, enrHandler)
    if err != nil {
        return nil, errors.Wrap(err, "unable to start the discv5 service")
    }

    return &Crawler{
        ctx:           ctx,
        ethNode:       ethNode,
        dbClient:      sqlDB,
        discv5Service: discv5Serv,
        enrCache:      enrCache,
    }, nil
}

func (c *Crawler) Run(duration time.Duration) error {
    // If no duration, run until Ctrl+C
    c.discv5Service.Run()
    return nil
}

func (c *Crawler) ID() string {
    return c.ethNode.ID().String()
}
