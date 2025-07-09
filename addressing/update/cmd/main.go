package main

import (
	"context"
	"net/http"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/addressing/update/client"
	"github.com/nuts-foundation/nuts-knooppunt/addressing/update/config"
)

func main() {
	println("Update client")

	cfg := config.NewExampleConfig()

	httpClient := http.Client{}
	updateClient := client.NewClient(httpClient)
	_, err := updateClient.RequestUpdates(context.Background(), cfg.MasterDirectory, time.Now())
	if err != nil {
		println("Error requesting updates:", err.Error())
		return
	}

	println("Local Directory:", cfg.LocalDirectory.Name, "at", cfg.LocalDirectory.Url.String())
}
