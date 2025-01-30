package main

import (
    "context"
    "fmt"
    "os"

    "github.com/sirupsen/logrus"
    cli "github.com/urfave/cli/v2"

    "github.com/example/crawler/cmd"
)

var (
    CliName    = "ethereum sepolia crawler"
    CliVersion = "v0.0.1"
)

func main() {
    fmt.Println(CliName, CliVersion)

    lightCrawler := cli.App{
        Name:      CliName,
        Usage:     "Discovers Ethereum (Sepolia) nodes via discv5 and saves them to a text file",
        UsageText: "crawler [subcommands] [arguments]",
        Commands: []*cli.Command{
            cmd.Discovery5,
        },
    }

    err := lightCrawler.RunContext(context.Background(), os.Args)
    if err != nil {
        logrus.Errorf("error running %s - %s", CliName, err.Error())
        os.Exit(1)
    }
}
