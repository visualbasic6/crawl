package cmd

import (
    "github.com/sirupsen/logrus"
    cli "github.com/urfave/cli/v2"

    "github.com/example/crawler/pkg/config"
    "github.com/example/crawler/pkg/crawler"
)

var Discovery5 = &cli.Command{
    Name:   "discv5",
    Usage:  "crawl Ethereum's public DHT using Discovery v5.1 (Sepolia)",
    Action: RunDiscv5,
    Flags: []cli.Flag{
        &cli.StringFlag{
            Name:  "log-level",
            Usage: "verbosity of the logs [debug,warn,info,error]",
            Value: "info",
        },
        &cli.IntFlag{
            Name:  "port",
            Usage: "UDP/TCP port to bind for discv5",
            Value: 9001,
        },
    },
}

func RunDiscv5(ctx *cli.Context) error {
    // parse the config
    conf := config.DefaultConfig
    conf.Apply(ctx)

    switch ctx.String("log-level") {
    case "debug":
        logrus.SetLevel(logrus.DebugLevel)
    case "warn":
        logrus.SetLevel(logrus.WarnLevel)
    case "error":
        logrus.SetLevel(logrus.ErrorLevel)
    default:
        logrus.SetLevel(logrus.InfoLevel)
    }

    // Create the crawler with no DB usage
    crawlr, err := crawler.New(
        ctx.Context,
        "",         // dbEndpoint not used anymore
        "sepolia_peerstore.db", // local enode DB file
        conf.UDP,
        false,      // resetDB not used
    )
    if err != nil {
        return err
    }

    logrus.WithFields(logrus.Fields{
        "peerID":   crawlr.ID(),
        "UDP":      conf.UDP,
    }).Info("Starting discv5 crawler for Sepolia")

    // run the crawler until Ctrl+C or context done
    return crawlr.Run(conf.CrawlDuration)
}
