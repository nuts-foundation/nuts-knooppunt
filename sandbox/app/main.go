package main

import (
	"log"
	"net/http"
	"os"
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
	if err := http.ListenAndServe(":"+port, NewMux(cfg)); err != nil {
		log.Fatal(err)
	}
}
