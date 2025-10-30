package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/nutsnode"
	"github.com/nuts-foundation/nuts-knooppunt/component/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
)

type Config struct {
	MCSD      mcsd.Config      `koanf:"mcsd"`
	MCSDAdmin mcsdadmin.Config `koanf:"mcsdadmin"`
	Nuts      nutsnode.Config  `koanf:"nuts"`
	NVI       nvi.Config       `koanf:"nvi"`
	PDP       pdp.Config       `koanf:"pdp"`
	MITZ      mitz.Config      `koanf:"mitz"`
}

func DefaultConfig() Config {
	return Config{
		Nuts: nutsnode.Config{
			Enabled: false,
		},
		MCSDAdmin: mcsdadmin.Config{},
		NVI:       nvi.DefaultConfig(),
		PDP:       pdp.DefaultConfig(),
		MITZ:      mitz.Config{},
	}
}

// LoadConfig loads configuration from YAML file and environment variables
func LoadConfig() (Config, error) {
	// Initialize koanf instance
	k := koanf.New(".")

	// Load default configuration first
	defaultConfig := DefaultConfig()
	if err := k.Load(structs.Provider(defaultConfig, "koanf"), nil); err != nil {
		return Config{}, err
	}

	// Try config files in config directory only
	configFiles := []string{"config/knooppunt.yml"}

	for _, cf := range configFiles {
		if _, err := os.Stat(cf); err == nil {
			if err := k.Load(file.Provider(cf), yaml.Parser()); err != nil {
				return Config{}, fmt.Errorf("failed to load config file %s: %w", cf, err)
			}
			break
		}
	}

	// Load environment variables with KNPT_ prefix
	if err := k.Load(env.Provider("KNPT_", ".", func(s string) string {
		// Convert KNPT_MCSD_LOCALDIRECTORY_FHIRBASEURL to mcsd.localdirectory.fhirbaseurl
		// First remove the prefix and convert to lowercase
		key := strings.TrimPrefix(s, "KNPT_")
		parts := strings.Split(key, "_")

		// Convert to lowercase path
		result := make([]string, len(parts))
		for i, part := range parts {
			result[i] = strings.ToLower(part)
		}
		return strings.Join(result, ".")
	}), nil); err != nil {
		return Config{}, err
	}

	// Unmarshal into config struct
	var config Config
	if err := k.Unmarshal("", &config); err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}
