package eth

import "errors"

type Config struct {
	Addr                  string   `toml:"addr" json:"addr"`
	DefaultGasLimit       uint64   `toml:"defaultGasLimit" json:"defaultGasLimit"`
	TransactionsPerSecond int      `toml:"transactionsPerSecond" json:"transactionsPerSecond"`
	Headers               []Header `toml:"headers" json:"headers"`
}

type Header struct {
	Key   string `toml:"key" json:"key"`
	Value string `toml:"value" json:"value"`
}

func (c *Config) Init() error {
	if c.Addr == "" {
		return errors.New("domain cannot be set to empty")
	}
	if c.DefaultGasLimit > 0 && c.DefaultGasLimit < 21000 {
		return errors.New("gasLimit too low")
	}
	if c.TransactionsPerSecond < 1 {
		c.TransactionsPerSecond = 1
	}
	return nil
}
