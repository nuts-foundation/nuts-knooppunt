package cmd

import (
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/component/nutsnode"
)

type Config struct {
	MCSD mcsd.Config
	Nuts nutsnode.Config
}

func DefaultConfig() Config {
	return Config{
		Nuts: nutsnode.Config{
			Enabled: true,
		},
	}
}
