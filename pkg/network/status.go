package discv5

import (
    "context"
    "crypto/ecdsa"
    "errors"
    "net"

    gethlog "github.com/ethereum/go-ethereum/log"
    "github.com/ethereum/go-ethereum/p2p/discover"
    "github.com/ethereum/go-ethereum/p2p/enode"
)

type Discv5Service struct {
    ctx         context.Context
    ethNode     *enode.LocalNode
    dv5Listener *discover.UDPv5
    enrHandler  func(*enode.Node)
}

func NewService(
    ctx context.Context,
    port int,
    privkey *ecdsa.PrivateKey,
    ethNode *enode.LocalNode,
    bootnodes []*enode.Node,
    enrHandler func(*enode.Node)) (*Discv5Service, error) {

    if len(bootnodes) == 0 {
        return nil, errors.New("no bootnodes provided")
    }

    udpAddr := &net.UDPAddr{
        IP:   net.IPv4zero,
        Port: port,
    }

    conn, err := net.ListenUDP("udp", udpAddr)
    if err != nil {
        return nil, err
    }

    gethLogger := gethlog.New()
    gethLogger.SetHandler(gethlog.DiscardHandler())

    cfg := discover.Config{
        PrivateKey:   privkey,
        NetRestrict:  nil,
        Bootnodes:    bootnodes,
        Log:          gethLogger,
        ValidSchemes: enode.ValidSchemes,
    }

    dv5Listener, err := discover.ListenV5(conn, ethNode, cfg)
    if err != nil {
        return nil, err
    }

    return &Discv5Service{
        ctx:         ctx,
        ethNode:     ethNode,
        dv5Listener: dv5Listener,
        enrHandler:  enrHandler,
    }, nil
}

func (dv5 *Discv5Service) Run() {
    for {
        // Check context first
        if err := dv5.ctx.Err(); err != nil {
            return
        }

        iterator := dv5.dv5Listener.RandomNodes()
        for iterator.Next() {
            newNode := iterator.Node()
            dv5.enrHandler(newNode)
        }
    }
}

// New method to handle discovered peers
func (dv5 *Discv5Service) HandlePeer(node *enode.Node) {
    if dv5.enrHandler != nil {
        dv5.enrHandler(node)
    }
}
