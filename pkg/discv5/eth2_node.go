package discv5

import (
    "crypto/ecdsa"
    "net"
    "time"

    "github.com/ethereum/go-ethereum/p2p/enode"
    "github.com/migalabs/armiarma/src/utils"
    "github.com/protolambda/zrnt/eth2/beacon/common"
)

// Minimal ENR wrapper
type EnrNode struct {
    Timestamp time.Time
    ID        enode.ID
    IP        net.IP
    Seq       uint64
    UDP       int
    TCP       int
    Pubkey    *ecdsa.PublicKey
    Eth2Data  *common.Eth2Data
    Attnets   *Attnets
}

func NewEnrNode(nodeID enode.ID) *EnrNode {
    return &EnrNode{
        Timestamp: time.Now(),
        ID:        nodeID,
        Pubkey:    new(ecdsa.PublicKey),
        Eth2Data:  new(common.Eth2Data),
        Attnets:   new(Attnets),
    }
}

type Attnets struct {
    Raw       utils.AttnetsENREntry
    NetNumber int
}
