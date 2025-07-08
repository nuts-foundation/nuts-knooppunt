package main

import (
	"github.com/nuts-foundation/nuts-knooppunt/addressing/update/config"
)

func main() {
	println("Update client")

	cfg := config.NewExampleConfig()

	println("Local Directory:", cfg.LocalDirectory.Name, "at", cfg.LocalDirectory.Url.String())
}
