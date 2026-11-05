package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"

	"github.com/nuts-foundation/nuts-knooppunt/lib/certutil"
)

// LoadClientCertificate loads a client certificate from PEM or PKCS#12 file (see certutil.Load).
func LoadClientCertificate(certFile, keyFile, password string) (tls.Certificate, error) {
	keyPair, err := certutil.Load(certFile, keyFile, password)
	if err != nil {
		return tls.Certificate{}, err
	}
	slog.Info("Loaded client certificate", slog.String("certFile", certFile))
	return tls.Certificate{
		Certificate: keyPair.Chain,
		PrivateKey:  keyPair.PrivateKey,
		Leaf:        keyPair.Leaf,
	}, nil
}

// LoadCACertPool loads CA certificates from file
func LoadCACertPool(caFile string) (*x509.CertPool, error) {
	if caFile == "" {
		return nil, nil
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	slog.Info("Loaded CA certificate", slog.String("caFile", caFile))
	return caCertPool, nil
}

type Config struct {
	// TLSCertFile is the PEM certificate file OR .p12/.pfx file
	TLSCertFile string `koanf:"tlscertfile"`
	// TLSKeyFile is the PEM key file (not used if TLSCertFile is .p12/.pfx)
	TLSKeyFile string `koanf:"tlskeyfile"`
	// TLSKeyPassword is the password for encrypted key or .p12/.pfx file
	TLSKeyPassword string `koanf:"tlskeypassword"`
	// TLSCAFile is the CA certificate file (or bundle) used to verify server/peer certificates
	TLSCAFile string `koanf:"tlscafile"`
}

// CreateTLSConfig creates a TLS configuration with client certificate and optional CA
func CreateTLSConfig(cfg Config) (*tls.Config, error) {
	cert, err := LoadClientCertificate(cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSKeyPassword)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// Load CA certificate if specified
	if cfg.TLSCAFile != "" {
		caCertPool, err := LoadCACertPool(cfg.TLSCAFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}
