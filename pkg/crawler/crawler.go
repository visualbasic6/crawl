package crawler

import (
    "context"
    "fmt"
    "sync"
    "net"
    "time"
    "crypto/ecdsa"
    "bufio"
    "os"

    "github.com/ethereum/go-ethereum/p2p/enode"
    "github.com/pkg/errors"
    "github.com/migalabs/eth-light-crawler/pkg/config"
    "github.com/migalabs/eth-light-crawler/pkg/discv5"
    ut "github.com/migalabs/eth-light-crawler/pkg/utils"
    log "github.com/sirupsen/logrus"
)

type Crawler struct {
    ctx           context.Context
    ethNode       *enode.LocalNode
    discv5Service *discv5.Discv5Service
    nodes         struct {
        sync.RWMutex
        addrs map[string]bool
    }
    outputFile    *os.File
    totalFound    int64
    totalMutex    sync.RWMutex
    threads       int
    privKey       *ecdsa.PrivateKey
}

const (
    checkInterval = 1000 // Clean duplicates every N nodes
)

func (c *Crawler) ID() string {
    return c.ethNode.ID().String()
}

func New(ctx context.Context, port int) (*Crawler, error) {
    log.SetLevel(log.DebugLevel)

    var threads int
    fmt.Print("Enter number of discovery threads: ")
    fmt.Scanf("%d", &threads)
    if threads < 1 {
        threads = 1
    }

    outputFile, err := os.Create("sepolia_peers.txt")
    if err != nil {
        return nil, fmt.Errorf("failed to create output file: %v", err)
    }

    privK, err := ut.GenNewPrivKey()
    if err != nil {
        return nil, errors.Wrap(err, "error generating privkey")
    }

    enodeDB, err := enode.OpenDB("nodes.db")
    if err != nil {
        return nil, err
    }

    ethNode := enode.NewLocalNode(enodeDB, privK)

    c := &Crawler{
        ctx:           ctx,
        ethNode:       ethNode,
        outputFile:    outputFile,
        threads:       threads,
        privKey:       privK,
    }
    c.nodes.addrs = make(map[string]bool)

    // Start progress monitor
    go c.monitorProgress()

    enrHandler := func(node *enode.Node) {
        if err := node.ValidateComplete(); err != nil {
            return
        }

        tcp := node.TCP()
        if tcp == 0 {
            return
        }

        ip := node.IP()
        if ip == nil {
            return
        }

        addr := fmt.Sprintf("enode://%s@%s:%d", node.ID().String(), ip.String(), tcp)

        c.nodes.RLock()
        seen := c.nodes.addrs[addr]
        c.nodes.RUnlock()
        if seen {
            return
        }

        // Fast validation - just try to connect
        if tcp == 30303 {
            if conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip.String(), tcp), 2*time.Second); err == nil {
                conn.Close()
                // Immediately try to get peers from this node
                go c.connectAndGetPeers(node)
            }
        }

        // Save the node
        c.nodes.Lock()
        if !c.nodes.addrs[addr] {
            c.nodes.addrs[addr] = true
            c.outputFile.WriteString(addr + "\n")

            c.totalMutex.Lock()
            c.totalFound++
            if c.totalFound%checkInterval == 0 {
                go c.removeDuplicates()
            }
            c.totalMutex.Unlock()
        }
        c.nodes.Unlock()
    }

    discv5Serv, err := discv5.NewService(ctx, port, privK, ethNode, config.EthBootonodes, enrHandler)
    if err != nil {
        return nil, errors.Wrap(err, "unable to generate the discv5 service")
    }

    c.discv5Service = discv5Serv
    return c, nil
}

func (c *Crawler) monitorProgress() {
    startTime := time.Now()
    ticker := time.NewTicker(50 * time.Millisecond)
    defer ticker.Stop()

    fmt.Printf("\033[s") // Save cursor position
    for range ticker.C {
        c.totalMutex.RLock()
        nodesPerMin := float64(c.totalFound) / time.Since(startTime).Minutes()
        fmt.Printf("\033[u\033[2K\rTotal nodes found: %d, nodes/minute: %.2f", c.totalFound, nodesPerMin)
        c.totalMutex.RUnlock()
    }
}

func (c *Crawler) connectAndGetPeers(node *enode.Node) {
    // Basic TCP handshake then immediately queue new discoveries
    if conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", node.IP().String(), node.TCP()), 2*time.Second); err == nil {
        defer conn.Close()
        // Queue this node for rediscovery after a short delay
        go func() {
            time.Sleep(5 * time.Second)
            if c.discv5Service != nil && node != nil {
                // Just pass the node through discovery again
                c.discv5Service.Run()
            }
        }()
    }
}

func (c *Crawler) removeDuplicates() {
    // Read all nodes
    uniqueNodes := make(map[string]bool)

    c.outputFile.Seek(0, 0)
    scanner := bufio.NewScanner(c.outputFile)
    for scanner.Scan() {
        uniqueNodes[scanner.Text()] = true
    }

    // Rewrite file with unique nodes
    c.outputFile.Truncate(0)
    c.outputFile.Seek(0, 0)
    for addr := range uniqueNodes {
        c.outputFile.WriteString(addr + "\n")
    }
}

func (c *Crawler) Run() error {
    log.Infof("Starting discovery service with %d threads...", c.threads)

    var wg sync.WaitGroup
    for i := 0; i < c.threads; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            c.discv5Service.Run()
        }(i)
    }

    wg.Wait()
    return nil
}
