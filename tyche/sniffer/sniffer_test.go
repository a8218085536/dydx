package sniffer

import (
	"testing"
)

func TestRegisterEvent(t *testing.T) {

	var defaultCfg = &Config{
		SecrityHeight:  0,
		NumberOfBlocks: 64,
	}

	var sniffer, err = New(defaultCfg)
	if err != nil {
		panic(err)
	}

	sniffer.SetEventHandler(func(e *Event) error {
		return nil
	})
}
