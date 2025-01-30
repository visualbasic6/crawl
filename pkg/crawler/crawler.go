package crawler

import (
    "context"
    "encoding/hex"
    "fmt"
    "os"
    "sync"
    "time"

    "github.com/ethereum/go-ethereum/crypto"
    "github.com/ethereum/go-ethereum/p2p/enode"
    "github.com/migalabs/armiarma/src/utils"
    "github.com/migalabs/eth-light-crawler/pkg/config"
    "github.com/migalabs/eth-light-crawler/pkg/discv5"
    ut "github.com/migalabs/eth-light-crawler/pkg/utils"

    "github.com/pkg/errors"
    log "github.com/sirupsen/logrus"
    "github.com/protolambda/zrnt/eth2/beacon/common"
)

type Crawler struct {
    ctx context.Context

    startT   time.Time
    duration time.Duration

    ethNode       *enode.LocalNode
    discv5Service *discv5.Discv5Service
    enrCache      map[enode.ID]uint64
}

// -----------------------------------------------------------------------------
// We'll define a global file + mutex for concurrency-safe writes
// -----------------------------------------------------------------------------

var outputMutex sync.Mutex
var outputFile = "sepolia_peers.txt"

// saveFullEnode writes a single line to `sepolia_peers.txt` in the format:
// enode://<64-byte-hex-pubkey>@<ip>:<port>
func saveFullEnode(node *enode.Node) {
    // (1) Grab uncompressed pubkey (65 bytes: 0x04 + 64)
    pubBytes := crypto.FromECDSAPub(node.Pubkey())
    // (2) Skip the 0x04 prefix
    pubHex := hex.EncodeToString(pubBytes[1:])

    // (3) Format the line
    line := fmt.Sprintf("enode://%s@%s:%d\n", pubHex, node.IP(), node.TCP())

    outputMutex.Lock()
    defer outputMutex.Unlock()

    f, err := os.OpenFile(outputFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    if err != nil {
        log.Errorf("Error opening %s: %v", outputFile, err)
        return
    }
    defer f.Close()

    if _, err := f.WriteString(line); err != nil {
        log.Errorf("Error writing enode line: %v", err)
    }
}

// -----------------------------------------------------------------------------
// The main constructor
// -----------------------------------------------------------------------------

func New(
    ctx context.Context,
    dbEndpoint string, // not used
    dbPath string,
    port int,
    resetDB bool, // not used
) (*Crawler, error) {

    // (A) Generate ephemeral private key for local node
    privK, err := ut.GenNewPrivKey()
    if err != nil {
        return nil, errors.Wrap(err, "error generating privkey")
    }

    // (B) enode DB => for storing local enode info (not the remote peers)
    enodeDB, err := enode.OpenDB(dbPath)
    if err != nil {
        return nil, err
    }

    // create local node
    ethNode := enode.NewLocalNode(enodeDB, privK)

    // track known Node IDs
    enrCache := make(map[enode.ID]uint64)

    // define how we handle newly discovered nodes
    enrHandler := func(node *enode.Node) {
        // basic sanity check
        err := node.ValidateComplete()
        if err != nil {
            log.Warnf("error validating the ENR: %s", err.Error())
        }

        id := node.ID()
        seq := node.Seq()
        ip := node.IP()
        tcp := node.TCP()
        udp := node.UDP()

        // parse optional data
        eth2Data, ok, parseErr := utils.ParseNodeEth2Data(*node)
        if parseErr != nil {
            // fallback
            eth2Data = new(common.Eth2Data)
        }
        if !ok {
            eth2Data = new(common.Eth2Data)
        }

        attnets, ok, attErr := discv5.ParseAttnets(*node)
        if attErr != nil {
            attnets = new(discv5.Attnets)
        }
        if !ok {
            attnets = new(discv5.Attnets)
        }

        // build an EnrNode struct (just for logging, if desired)
        enrNode := discv5.NewEnrNode(id)
        enrNode.Seq = seq
        enrNode.IP = ip
        enrNode.TCP = tcp
        enrNode.UDP = udp
        enrNode.Eth2Data = eth2Data
        enrNode.Attnets = attnets

        // log to console
        log.WithFields(log.Fields{
            "node_id":     id,
            "ip":          ip,
            "tcp":         tcp,
            "udp":         udp,
        }).Info("Discovered node on Sepolia")

        // If new or updated ENR
        prevSeq, found := enrCache[id]
        if !found || seq > prevSeq {
            // Write to file as a fully valid enode
            saveFullEnode(node)
            enrCache[id] = seq
        }
    }

    // (C) Start discv5
    discv5Serv, err := discv5.NewService(ctx, port, privK, ethNode, config.EthBootonodes, enrHandler)
    if err != nil {
        return nil, errors.Wrap(err, "unable to start discv5 service")
    }

    return &Crawler{
        ctx:           ctx,
        ethNode:       ethNode,
        discv5Service: discv5Serv,
        enrCache:      enrCache,
    }, nil
}

// -----------------------------------------------------------------------------
// Run forever or until context done
// -----------------------------------------------------------------------------

func (c *Crawler) Run(duration time.Duration) error {
    // if duration is 0, we just run until Ctrl+C
    c.discv5Service.Run()
    return nil
}

// -----------------------------------------------------------------------------
// Return local node ID
// -----------------------------------------------------------------------------

func (c *Crawler) ID() string {
    return c.ethNode.ID().String()
}
