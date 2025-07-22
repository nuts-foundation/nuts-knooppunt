package main

import (
	"context"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/addressing/update/client"
	"github.com/nuts-foundation/nuts-knooppunt/addressing/update/config"
)

func main() {
	println("Update client")

	cfg := config.NewExampleConfig()

	httpClient := http.Client{}
	updateClient := client.NewClient(cfg, &httpClient)
	err := updateClient.SyncMasterDirectory(context.Background())
	if err != nil {
		println("Error requesting updates:", err.Error())
		return
	}

	println("Local Directory:", cfg.LocalDirectory.Name, "at", cfg.LocalDirectory.Url.String())
}
