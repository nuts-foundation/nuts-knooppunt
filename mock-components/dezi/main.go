package main

import (
	"log"
	"net/http"
	"os"
	"strings"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	var (
		httpPort = env("PORT", "8092")
		tlsPort  = env("TLS_PORT", "8443")
		certFile = env("TLS_CERT_FILE", "")
		keyFile  = env("TLS_KEY_FILE", "")
		issuer   = env("DEZI_ISSUER", "https://abonnee.dezi.nl")
		// The Nuts node fetches this URL to verify the attestation signature.
		// It must be HTTPS and must appear in the node's vcr.dezi.allowedjku.
		jku = env("DEZI_JKU", "https://mock-dezi:8443/dezi/jwks.json")
		// Empty means "generate in memory", which is what `go run` uses.
		keyFileForSigning = env("DEZI_SIGNING_KEY_FILE", "")
		// The callbacks /authorize will bind, comma separated. Matched exactly:
		// without it this mock redirects a browser wherever a link asks, which
		// on a hosted sandbox lends out the deployment's own domain.
		allowedRedirectURIs = strings.Split(env("DEZI_ALLOWED_REDIRECT_URIS", "http://localhost:8091/demo/auth/callback"), ",")
	)
	for i := range allowedRedirectURIs {
		allowedRedirectURIs[i] = strings.TrimSpace(allowedRedirectURIs[i])
	}

	key, err := loadOrGenerateKey(keyFileForSigning)
	if err != nil {
		log.Fatalf("signing key: %v", err)
	}
	if keyFileForSigning == "" {
		log.Print("mock-dezi: signing key generated in memory, it changes on every restart")
	}
	mux := NewMux(NewSigner(key, env("DEZI_KEY_ID", "sandbox-dezi-1"), issuer, jku), DrElAmrani, allowedRedirectURIs)

	if certFile != "" && keyFile != "" {
		go func() {
			log.Printf("mock-dezi starting TLS listener on :%s (jku %s)", tlsPort, jku)
			if err := http.ListenAndServeTLS(":"+tlsPort, certFile, keyFile, mux); err != nil {
				log.Fatalf("tls listener: %v", err)
			}
		}()
	} else {
		log.Print("mock-dezi: TLS disabled, the Nuts node will not be able to fetch the jku")
	}

	log.Printf("mock-dezi listening on :%s", httpPort)
	if err := http.ListenAndServe(":"+httpPort, mux); err != nil {
		log.Fatal(err)
	}
}
