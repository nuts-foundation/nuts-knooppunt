package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}

	cfg, err := NewConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	if !cfg.configured() {
		log.Printf("gf-sandbox: reset/recycle disabled (set KNOOPPUNT_INTERNAL_URL and HAPI_BASE_URL to enable)")
	}

	log.Printf("gf-sandbox listening on :%s", port)
	server := &http.Server{
		Addr:    ":" + port,
		Handler: NewMux(cfg),
		// WriteTimeout is the loosest of the three: the share POST waits on the
		// NVI and Mitz, each already bounded well inside it.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
