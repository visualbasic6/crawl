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
    iterator    enode.Iterator
    enrHandler  func(*enode.Node)
}

func NewService(
    ctx context.Context,
    port int,
    privkey *ecdsa.PrivateKey,
    ethNode *enode.LocalNode,
    bootnodes []*enode.Node,
    enrHandler func(*enode.Node),
) (*Discv5Service, error) {

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
    gethLogger.SetHandler(gethlog.FuncHandler(func(r *gethlog.Record) error {
        // No-op or debug
        return nil
    }))

    cfg := discover.Config{
        PrivateKey:   privkey,
        Bootnodes:    bootnodes,
        ValidSchemes: enode.ValidSchemes,
        Log:          gethLogger,
    }

    dv5Listener, err := discover.ListenV5(conn, ethNode, cfg)
    if err != nil {
        return nil, err
    }

    iter := dv5Listener.RandomNodes()
    return &Discv5Service{
        ctx:         ctx,
        ethNode:     ethNode,
        dv5Listener: dv5Listener,
        iterator:    iter,
        enrHandler:  enrHandler,
    }, nil
}

func (dv5 *Discv5Service) Run() {
    for {
        if err := dv5.ctx.Err(); err != nil {
            break
        }
        if dv5.iterator.Next() {
            newNode := dv5.iterator.Node()
            dv5.enrHandler(newNode)
        }
    }
}
