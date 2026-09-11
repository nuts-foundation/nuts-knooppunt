// Command mock-prs runs the mock PRS as a standalone service for docker
// compose: the PRS API on LISTEN_ADDR (default ":8080"), in plain HTTP,
// because mTLS is terminated in front of the service, and known to its
// clients as PUBLIC_URL (required, such as "http://mock-prs:8080": the
// audience the token endpoint demands and the target its tokens are issued
// for), and, only when SANDBOX_LISTEN_ADDR is set, the sandbox helper on
// that address. The helper has no authentication: whoever reaches it can turn any
// identifier value into its pseudonym and mint identifier values for any
// pseudonym, so it stays off unless the network is closed, as it is inside
// the compose project, which sets ":8080" and publishes no port for it.
//
// RECIPIENT_KEY_FILE holds the PEM RSA private key every JWE is encrypted to,
// OPRF_KEY_FILE the OPRF key as 32 bytes in hex; without them both are
// generated, and every pseudonym changes on the next restart. RECIPIENTS
// lists the organizations and scopes with a registered key as "ura:scope"
// pairs separated by commas (default "90000901:nationale-verwijsindex").
package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nuts-foundation/nuts-knooppunt/mock-components/prs"
)

func main() {
	if err := run(); err != nil {
		slog.Error("mock PRS failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	opts := prsmock.Options{
		ListenAddr:        envOr("LISTEN_ADDR", ":8080"),
		SandboxListenAddr: os.Getenv("SANDBOX_LISTEN_ADDR"),
		PublicURL:         os.Getenv("PUBLIC_URL"),
	}
	if opts.PublicURL == "" {
		return errors.New("PUBLIC_URL must be set to the origin clients use for this service, such as http://mock-prs:8080")
	}
	if opts.SandboxListenAddr != "" {
		slog.Warn("sandbox helper enabled; it de-tokenizes and mints for anyone who can reach it", "addr", opts.SandboxListenAddr)
	}

	if file := os.Getenv("RECIPIENT_KEY_FILE"); file != "" {
		key, err := loadRSAPrivateKey(file)
		if err != nil {
			return fmt.Errorf("loading RECIPIENT_KEY_FILE: %w", err)
		}
		opts.RecipientKey = key
	} else {
		slog.Warn("RECIPIENT_KEY_FILE not set; generating a recipient key nobody else holds")
	}

	if file := os.Getenv("OPRF_KEY_FILE"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("loading OPRF_KEY_FILE: %w", err)
		}
		key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return fmt.Errorf("OPRF_KEY_FILE is not hex: %w", err)
		}
		opts.OPRFKey = key
	} else {
		slog.Warn("OPRF_KEY_FILE not set; generating an OPRF key, so pseudonyms change on the next restart")
	}

	service, err := prsmock.New(opts)
	if err != nil {
		return err
	}
	for _, recipient := range strings.Split(envOr("RECIPIENTS", "90000901:nationale-verwijsindex"), ",") {
		ura, scope, found := strings.Cut(strings.TrimSpace(recipient), ":")
		if !found || ura == "" || scope == "" {
			return fmt.Errorf("RECIPIENTS entry %q is not ura:scope", recipient)
		}
		service.RegisterRecipient(ura, scope)
		slog.Info("registered recipient", "ura", ura, "scope", scope)
	}
	slog.Info("mock PRS listening", "prs", service.GetURL(), "sandbox", service.SandboxURL())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	sig := <-signals
	slog.Info("shutting down", "signal", sig.String())
	return service.Stop()
}

func envOr(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// loadRSAPrivateKey reads a PEM RSA private key in PKCS#8 or PKCS#1 form, the
// two encodings openssl genrsa produces.
func loadRSAPrivateKey(file string) (*rsa.PrivateKey, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA key: %T", key)
		}
		return rsaKey, nil
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
