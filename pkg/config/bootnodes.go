package config

import (
    "github.com/ethereum/go-ethereum/p2p/enode"
)

// Official Sepolia bootnodes from Geth:
var EthBootonodes = []*enode.Node{
    enode.MustParse("enode://c6181e0534ffffe326beb3f55d0c42b43c07caaad18ae2d5b747fb7bbd5d6a8a5092d3e7715d167e7b1c1b2a4ee18f7105e1c17f35fdb2ce5ef021e6718102c7@147.182.222.6:30303"),
}
