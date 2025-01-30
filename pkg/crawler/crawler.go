package crawler

import (
    "context"
    "encoding/hex"
    "fmt"
    "os"
    "sync"
    "time"

    "github.com/migalabs/armiarma/src/utils"
    "github.com/migalabs/eth-light-crawler/pkg/config"
    "github.com/migalabs/eth-light-crawler/pkg/discv5"
    ut "github.com/migalabs/eth-light-crawler/pkg/utils"

    "github.com/pkg/errors"

    "github.com/ethereum/go-ethereum/p2p/enode"
    "github.com/protolambda/zrnt/eth2/beacon/common"
    log "github.com/sirupsen/logrus"
)

type Crawler struct {
    ctx context.Context

    startT   time.Time
    duration time.Duration

    ethNode       *enode.LocalNode
    discv5Service *discv5.Discv5Service

    enrCache map[enode.ID]uint64
}

// ---------------------------------------------------
// ### ADDED FILE LOGIC ###
// We'll define a global mutex & function for line-based file writes
// ---------------------------------------------------
var fileMutex sync.Mutex
var outputFilename = "sepolia_peers.txt"

// Write the discovered node to a text file with | delimiter
func saveENRToFile(enr *discv5.EnrNode) {
    // Build a single line. You can include as many fields as you want:
    // For example: node_id|ip|tcp|udp|fork_digest|timestamp...
    // And so on, depending on what you want to store.
    line := fmt.Sprintf(
        "%d|%s|%s|%d|%d|%d|%s|%s\n",
        enr.Timestamp.Unix(),
        enr.ID.String(),
        enr.IP.String(),
        enr.TCP,
        enr.UDP,
        enr.Seq,
        hex.EncodeToString(enr.Attnets.Raw[:]),
        enr.Eth2Data.ForkDigest.String(),
    )

    fileMutex.Lock()
    defer fileMutex.Unlock()

    f, err := os.OpenFile(outputFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    if err != nil {
        log.Errorf("Error opening output file: %v", err)
        return
    }
    defer f.Close()

    if _, err := f.WriteString(line); err != nil {
        log.Errorf("Error writing ENR line: %v", err)
    }
}

// ---------------------------------------------------

func New(
    ctx context.Context,
    dbEndpoint string, // not used now
    dbPath string,
    port int,
    resetDB bool, // not used
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

    // Generate local enode
    ethNode := enode.NewLocalNode(enodeDB, privK)

    enrCache := make(map[enode.ID]uint64)

    // define how we handle newly discovered ENRs
    enrHandler := func(node *enode.Node) {
        // Basic data parse
        err := node.ValidateComplete()
        if err != nil {
            log.Warnf("error validating ENR: %s", err.Error())
        }

        id := node.ID()
        seq := node.Seq()
        ip := node.IP()
        udp := node.UDP()
        tcp := node.TCP()
        pubkey := node.Pubkey()

        // parse eth2 data
        eth2Data, ok, parseErr := utils.ParseNodeEth2Data(*node)
        if parseErr != nil {
            log.Warnf("eth2 data parse error on node %s: %v", id, parseErr)
            eth2Data = new(common.Eth2Data)
        }
        if !ok {
            eth2Data = new(common.Eth2Data)
        }

        // parse attnets
        attnets, ok, attErr := discv5.ParseAttnets(*node)
        if attErr != nil {
            log.Warnf("attnets parse error on node %s: %v", id, attErr)
            attnets = new(discv5.Attnets)
        }
        if !ok {
            attnets = new(discv5.Attnets)
        }

        enrNode := discv5.NewEnrNode(id)
        enrNode.Seq = seq
        enrNode.IP = ip
        enrNode.TCP = tcp
        enrNode.UDP = udp
        enrNode.Pubkey = pubkey
        enrNode.Eth2Data = eth2Data
        enrNode.Attnets = attnets

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

        // Instead of storing to DB, we just append to a file
        prevSeq, known := enrCache[enrNode.ID]
        if !known || enrNode.Seq > prevSeq {
            saveENRToFile(enrNode)
            enrCache[enrNode.ID] = enrNode.Seq
        }
    }

    // spin up discv5
    discv5Serv, err := discv5.NewService(ctx, port, privK, ethNode, config.EthBootonodes, enrHandler)
    if err != nil {
        return nil, errors.Wrap(err, "unable to start the discv5 service")
    }

    return &Crawler{
        ctx:           ctx,
        ethNode:       ethNode,
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
