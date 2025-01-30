package config

import (
    "time"

    cli "github.com/urfave/cli/v2"
)

// Basic config
type Config struct {
    UDP           int
    CrawlDuration time.Duration
}

var DefaultConfig = Config{
    UDP:           9001,
    CrawlDuration: 0, // 0 means run forever
}

func (c *Config) Apply(ctx *cli.Context) {
    if ctx.IsSet("port") {
        c.UDP = ctx.Int("port")
    }
}
