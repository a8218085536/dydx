package sniffer

import "errors"

type Config struct {
	SecrityHeight  uint64           `toml:"secrityHeight"`
	NumberOfBlocks uint64           `toml:"numberOfBlocks"`
	Callback       string           `toml:"callback"`
	Contracts      []ContractConfig `toml:"contracts"`
}

type ContractConfig struct {
	Addr   string   `toml:"addr"`
	ABI    string   `toml:"abi"`
	Events []string `toml:"events"`
}

func (c *Config) Init() error {
	if c.Callback == "" {
		return errors.New("callback cannot be set to empty")
	}

	if c.NumberOfBlocks < 16 {
		c.NumberOfBlocks = 16
	} else if c.NumberOfBlocks > 512 {
		c.NumberOfBlocks = 512
	}
	return nil
}
