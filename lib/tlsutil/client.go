package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"software.sslmate.com/src/go-pkcs12"
)

// Config holds TLS configuration options
type Config struct {
	CertFile string // PEM certificate file OR .p12/.pfx file
	KeyFile  string // PEM key file (not used if CertFile is .p12/.pfx)
	Password string // Password for encrypted key or .p12/.pfx file
	CAFile   string // CA certificate file to verify server
}

// LoadClientCertificate loads a client certificate from PEM or PKCS#12 file
func LoadClientCertificate(config Config) (tls.Certificate, error) {
	if config.CertFile == "" {
		return tls.Certificate{}, fmt.Errorf("certificate file not specified")
	}

	// Check if it's PKCS#12 (.p12/.pfx)
	ext := strings.ToLower(filepath.Ext(config.CertFile))
	isPKCS12 := ext == ".p12" || ext == ".pfx"

	if isPKCS12 {
		cert, err := loadPKCS12(config.CertFile, config.Password)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("failed to load PKCS#12: %w", err)
		}
		log.Info().Str("p12File", config.CertFile).Msg("Loaded client certificate from PKCS#12")
		return cert, nil
	}

	// Load PEM
	if config.KeyFile == "" {
		return tls.Certificate{}, fmt.Errorf("key file required when using PEM certificate")
	}
	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load certificate: %w", err)
	}
	log.Info().Str("certFile", config.CertFile).Str("keyFile", config.KeyFile).Msg("Loaded client certificate from PEM")
	return cert, nil
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

	log.Info().Str("caFile", caFile).Msg("Loaded CA certificate")
	return caCertPool, nil
}

// CreateTLSConfig creates a TLS configuration with client certificate and optional CA
func CreateTLSConfig(config Config) (*tls.Config, error) {
	cert, err := LoadClientCertificate(config)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// Load CA certificate if specified
	if config.CAFile != "" {
		caCertPool, err := LoadCACertPool(config.CAFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// loadPKCS12 loads a certificate and key from PKCS#12 file
func loadPKCS12(p12File, password string) (tls.Certificate, error) {
	data, err := os.ReadFile(p12File)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to read PKCS#12 file: %w", err)
	}

	blocks, err := pkcs12.ToPEM(data, password)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to decode PKCS#12: %w", err)
	}

	// Extract cert and key from PEM blocks
	var certPEM, keyPEM []byte
	for _, block := range blocks {
		pemBytes := pem.EncodeToMemory(block)
		if block.Type == "CERTIFICATE" {
			certPEM = append(certPEM, pemBytes...)
		} else if strings.Contains(block.Type, "PRIVATE KEY") {
			keyPEM = pemBytes
		}
	}

	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return tls.Certificate{}, fmt.Errorf("certificate or key not found in PKCS#12")
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}
