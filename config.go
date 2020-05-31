package monitor

import "time"

type Config struct {
	FactomdURL      string
	IncludePartial  bool
	RetryInterval   time.Duration
	RetryMultiplier float64
	RetryMax        time.Duration
}

func DefaultConfiguration() *Config {
	c := new(Config)
	c.FactomdURL = "https://api.factomd.net/v2"
	c.RetryInterval = time.Millisecond * 50
	c.RetryMultiplier = 1.5
	c.RetryMax = time.Second * 15
	return c
}
