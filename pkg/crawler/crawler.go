package crawler

import (
    "bufio"
    "context"
    "crypto/ecdsa"
    "encoding/hex"
    "fmt"
    "net"
    "os"
    "strings"
    "sync"
    "time"

    "github.com/ethereum/go-ethereum/crypto"
    "github.com/ethereum/go-ethereum/p2p"
    "github.com/ethereum/go-ethereum/p2p/discover"
    "github.com/ethereum/go-ethereum/p2p/enode"
    "github.com/ethereum/go-ethereum/p2p/rlpx"
    "github.com/ethereum/go-ethereum/rlp"
    "github.com/migalabs/armiarma/src/utils"
    "github.com/migalabs/eth-light-crawler/pkg/config"
    "github.com/migalabs/eth-light-crawler/pkg/discv5"
    ut "github.com/migalabs/eth-light-crawler/pkg/utils"
    "github.com/pkg/errors"
    log "github.com/sirupsen/logrus"
    "github.com/protolambda/zrnt/eth2/beacon/common"
)

// -----------------------------------------------------------------------------
// Data structures to hold handshake info
// -----------------------------------------------------------------------------

type protoHandshake struct {
    Version    uint64
    Name       string
    Caps       []p2p.Cap
    ListenPort uint64
    ID         []byte
}

// -----------------------------------------------------------------------------
// The main crawler
// -----------------------------------------------------------------------------

type Crawler struct {
    ctx context.Context

    ethNode       *enode.LocalNode
    discv5Service *discv5.Discv5Service
    enrCache      map[enode.ID]uint64
    outMutex      sync.Mutex
}

var outputFilename = "sepolia_peers.txt"
var fileMutex sync.Mutex

func New(
    ctx context.Context,
    dbEndpoint string, // not used
    dbPath string,
    port int,
    resetDB bool, // not used
) (*Crawler, error) {

    privK, err := ut.GenNewPrivKey()
    if err != nil {
        return nil, errors.Wrap(err, "error generating privkey")
    }

    enodeDB, err := enode.OpenDB(dbPath)
    if err != nil {
        return nil, err
    }

    ethNode := enode.NewLocalNode(enodeDB, privK)
    enrCache := make(map[enode.ID]uint64)

    // discv5 callback
    enrHandler := func(node *enode.Node) {
        // if we already have a newer seq, skip
        prevSeq, found := enrCache[node.ID()]
        if found && node.Seq() <= prevSeq {
            return
        }
        enrCache[node.ID()] = node.Seq()

        // Now do a devp2p handshake check in a goroutine
        go verifySepoliaPeer(ctx, node)
    }

    // spin up discv5
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

func (c *Crawler) Run(duration time.Duration) error {
    // just run discv5
    c.discv5Service.Run()
    return nil
}

func (c *Crawler) ID() string {
    return c.ethNode.ID().String()
}

// -----------------------------------------------------------------------------
// The "verifySepoliaPeer" function: do RLPx handshake, check chain ID, parse user agent
// -----------------------------------------------------------------------------

func verifySepoliaPeer(ctx context.Context, node *enode.Node) {
    // make ephemeral key for RLPx
    ourKey, err := ut.GenNewPrivKey()
    if err != nil {
        return
    }

    // dial
    d := net.Dialer{Timeout: 5 * time.Second}
    conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", node.IP(), node.TCP()))
    if err != nil {
        // cannot connect => skip
        return
    }
    defer conn.Close()

    // set up RLPx
    rlpxConn := rlpx.NewConn(conn, node.Pubkey())
    if err := doHandshakes(ctx, rlpxConn, ourKey); err != nil {
        // handshake failed => skip
        // log.Debugf("handshake with %s failed: %v", node.URLv4(), err)
        return
    }
}

// -----------------------------------------------------------------------------
// Actually do the devp2p base handshake + ETH status check
// If it matches Sepolia, save "enode://...|clientName"
// -----------------------------------------------------------------------------

func doHandshakes(ctx context.Context, c *rlpx.Conn, ourKey *ecdsa.PrivateKey) error {
    // set a deadline
    deadline := time.Now().Add(5 * time.Second)
    if err := c.SetDeadline(deadline); err != nil {
        return err
    }
    defer c.SetDeadline(time.Time{}) // reset

    // 1) RLPx handshake
    _, err := c.Handshake(ourKey)
    if err != nil {
        return fmt.Errorf("rlpx handshake: %w", err)
    }

    // 2) devp2p base handshake
    pub := crypto.FromECDSAPub(&ourKey.PublicKey)[1:]
    ourHandshake := &protoHandshake{
        Version: 5,
        Name:    "Sepolia-Crawler/v1.0",
        Caps: []p2p.Cap{
            {Name: "eth", Version: 68}, // or 67, or 70, etc.
        },
        ListenPort: 30303,
        ID:         pub,
    }
    // Send
    if err := writeMsg(c, 0, ourHandshake); err != nil {
        return fmt.Errorf("send base handshake: %w", err)
    }
    // Read
    code, data, _, err := c.Read()
    if err != nil {
        return fmt.Errorf("read base handshake: %w", err)
    }
    if code != 0 {
        return fmt.Errorf("unexpected base handshake code %d, want 0", code)
    }

    var remoteHandshake protoHandshake
    if err := rlp.DecodeBytes(data, &remoteHandshake); err != nil {
        return fmt.Errorf("decode base handshake: %w", err)
    }

    // the remote "Name" is the user agent
    clientName := remoteHandshake.Name

    // snappy if version >= 5
    if remoteHandshake.Version >= 5 {
        c.SetSnappy(true)
    }

    // pick highest "eth" version
    var ethVersion uint64
    for _, cap := range remoteHandshake.Caps {
        if cap.Name == "eth" && uint64(cap.Version) > ethVersion {
            ethVersion = uint64(cap.Version)
        }
    }
    if ethVersion == 0 {
        return fmt.Errorf("peer doesn't support eth")
    }

    // 3) Send ETH status
    // for Sepolia chain ID = 11155111
    // genesis = 0x25a5cc106eea7138acab33231d7160d69cb777ee0c2c553fcddf5138993e6dd9
    // forkid = 0xb96cbd13 next=1677557088
    // TTD not strictly required if node doesn't enforce, but let's do it anyway
    type statusMsg struct {
        ProtocolVersion uint32
        NetworkID       uint64
        TD              *big.Int
        Head            [32]byte
        Genesis         [32]byte
        ForkID          struct {
            Hash [4]byte
            Next uint64
        }
    }
    var s statusMsg
    s.ProtocolVersion = uint32(ethVersion)
    s.NetworkID = 11155111
    s.TD = bigIntFromDecimal("17000000000000000") // Terminal TD for sepolia
    copy(s.Head[:], mustHexToBytes("0x25a5cc106eea7138acab33231d7160d69cb777ee0c2c553fcddf5138993e6dd9"))
    copy(s.Genesis[:], mustHexToBytes("0x25a5cc106eea7138acab33231d7160d69cb777ee0c2c553fcddf5138993e6dd9"))
    s.ForkID.Hash = [4]byte{0xb9, 0x6c, 0xbd, 0x13}
    s.ForkID.Next = 1677557088

    if err := writeMsg(c, ethVersionOffset(ethVersion, 0), s); err != nil {
        return fmt.Errorf("send eth status: %w", err)
    }

    // 4) read remote status
    code, data, _, err = c.Read()
    if err != nil {
        return fmt.Errorf("read remote status: %w", err)
    }
    expected := ethVersionOffset(ethVersion, 0)
    if code != expected {
        return fmt.Errorf("unexpected code %d, want %d", code, expected)
    }

    var remoteStatus struct {
        ProtocolVersion uint32
        NetworkID       uint64
        TD              *big.Int
        Head            [32]byte
        Genesis         [32]byte
        ForkID          struct {
            Hash [4]byte
            Next uint64
        }
    }
    if err := rlp.DecodeBytes(data, &remoteStatus); err != nil {
        return fmt.Errorf("decode remote status: %w", err)
    }

    // check if chain matches
    if remoteStatus.NetworkID != 11155111 {
        return fmt.Errorf("not on sepolia net, got chainid=%d", remoteStatus.NetworkID)
    }

    // If we get here => all checks out => store them
    // parse client name => e.g. "Geth/v1.14.7-stable" => we only want "geth"
    shortName := parseClientName(clientName)

    // build enode://<64-byte-pubkey>@<ip>:<port>|client
    // we can do it from the c's remote pubkey + remote address
    remotePub := c.RemotePubkey() // or from the handshake data
    remoteIP := c.RemoteAddr()
    if remotePub == nil || remoteIP == nil {
        return fmt.Errorf("no remote pub or IP?!")
    }

    pubBytes := crypto.FromECDSAPub(remotePub)[1:] // skip 0x04
    pubHex := hex.EncodeToString(pubBytes)

    host, portStr, err := net.SplitHostPort(remoteIP.String())
    if err != nil {
        return err
    }

    line := fmt.Sprintf("enode://%s@%s:%s|%s\n", pubHex, host, portStr, shortName)

    // append to file
    fileMutex.Lock()
    defer fileMutex.Unlock()
    f, err := os.OpenFile(outputFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    if err == nil {
        defer f.Close()
        w := bufio.NewWriter(f)
        w.WriteString(line)
        w.Flush()
    }

    return nil
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func parseClientName(name string) string {
    l := strings.ToLower(name)
    switch {
    case strings.Contains(l, "geth"):
        return "geth"
    case strings.Contains(l, "nethermind"):
        return "nethermind"
    case strings.Contains(l, "besu"):
        return "besu"
    case strings.Contains(l, "reth"):
        return "reth"
    default:
        // or try to parse out the first token
        parts := strings.Split(name, "/")
        if len(parts) > 0 {
            return parts[0]
        }
        return name
    }
}

func bigIntFromDecimal(dec string) *big.Int {
    // parse a decimal string
    bi := new(big.Int)
    bi.SetString(dec, 10)
    return bi
}

func mustHexToBytes(h string) []byte {
    d, _ := hex.DecodeString(strings.TrimPrefix(h, "0x"))
    return d
}

func writeMsg(c *rlpx.Conn, code uint64, msg interface{}) error {
    payload, err := rlp.EncodeToBytes(msg)
    if err != nil {
        return err
    }
    _, err = c.Write(code, payload)
    return err
}

func ethVersionOffset(v uint64, code uint64) uint64 {
    // base protocol length is 16
    // ETH subprotocol starts at offset 16
    return 16*1 + code
}
