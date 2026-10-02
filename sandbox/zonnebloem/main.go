// Command zonnebloem-ehr is Zorgcentrum De Zonnebloem's own record system in the
// GF Sandbox: the source side of the marker proof. See README.md.
package main

import (
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"time"
)

func main() {
	cfg, err := configFromEnv(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	st := newFHIRStore(cfg.fhirBaseURL, &http.Client{Timeout: 10 * time.Second})
	marker := func() string { return suggestedMarker(rand.IntN) }

	log.Printf("zonnebloem-ehr listening on :%s, store %s", cfg.port, cfg.fhirBaseURL)
	server := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           newMux(st, time.Now, marker),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
