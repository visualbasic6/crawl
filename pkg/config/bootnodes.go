package config

import (
    "github.com/ethereum/go-ethereum/p2p/enode"
)

var EthBootonodes = []*enode.Node{
    enode.MustParse("enode://c6181e0534ffffe326beb3f55d0c42b43c07caaad18ae2d5b747fb7bbd5d6a8a5092d3e7715d167e7b1c1b2a4ee18f7105e1c17f35fdb2ce5ef021e6718102c7@147.182.222.6:30303"),
    enode.MustParse("enode://9246d00bc8fd1742e5ad2428b80fc4dc45d786283e05ef6edbd9002cbc335d40998444732fbe921cb88e1d2c73d1b1de53bae6a2237996e9bfe14f871baf7066@18.168.182.86:30303"),
    enode.MustParse("enode://ec66ddcf1a974950bd4c782789a7e04f8aa7110a72569b6e65fcd51e937e74eed303b1ea734e4d19cfaec9fbff9b6ee65bf31dcb50ba79acce9dd63a6aca61c7@52.14.151.177:30303"),
}
