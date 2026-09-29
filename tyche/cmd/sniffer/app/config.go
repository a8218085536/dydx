package app

import (
	"ethgo/eth"
	"ethgo/model"
	"ethgo/sniffer"
	"ethgo/util/logx"
	"fmt"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Logger  *logx.Config    `toml:"logger"`
	Backend *eth.Config     `toml:"backend"`
	Redis   *model.Config   `toml:"redis"`
	Sniffer *sniffer.Config `toml:"sniffer"`
}

func NewConfig(filepath string) (*Config, error) {
	var c = new(Config)
	if _, err := toml.DecodeFile(filepath, c); err != nil {
		return nil, err
	}

	if err := c.Init(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) Init() error {
	if err := c.Backend.Init(); err != nil {
		return err
	}

	if c.Redis.Domain == "" {
		return fmt.Errorf("domain cannot be set to empty")
	}

	c.Redis.Namespace = c.Redis.Domain
	return c.Sniffer.Init()
}
