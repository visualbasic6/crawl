package cmd

import (
    "github.com/migalabs/eth-light-crawler/pkg/config"
    "github.com/migalabs/eth-light-crawler/pkg/crawler"

    log "github.com/sirupsen/logrus"
    cli "github.com/urfave/cli/v2"
)

var Discovery5 = &cli.Command{
    Name:   "discv5",
    Usage:  "crawl Ethereum's Sepolia network through the Discovery 5.1 protocol",
    Action: RunDiscv5,
    Flags: []cli.Flag{
        &cli.StringFlag{
            Name:    "log-level",
            Usage:   "verbosity of the logs that will be displayed [debug,warn,info,error]",
            EnvVars: []string{"IPFS_CID_HOARDER_LOGLEVEL"},
            Value:   "info",
        },
        &cli.IntFlag{
            Name:  "port",
            Usage: "port number that we want to use/advertise in the Ethereum network",
            Value: 30303,
        },
    },
}

func RunDiscv5(ctx *cli.Context) error {
    conf := config.DefaultConfig
    conf.Apply(ctx)

    crawlr, err := crawler.New(ctx.Context, conf.UDP)
    if err != nil {
        return err
    }

    log.WithFields(log.Fields{
        "peerID":    crawlr.ID(),
        "UDP":       conf.UDP,
        "bootnodes": len(config.EthBootonodes),
    }).Info("Starting Sepolia discovery node")

    return crawlr.Run()
}
