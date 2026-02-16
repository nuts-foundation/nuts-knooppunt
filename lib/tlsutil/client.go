package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"software.sslmate.com/src/go-pkcs12"
)

// LoadClientCertificate loads a client certificate from PEM or PKCS#12 file
func LoadClientCertificate(certFile, keyFile, password string) (tls.Certificate, error) {
	if certFile == "" {
		return tls.Certificate{}, fmt.Errorf("certificate file not specified")
	}

	// Check if it's PKCS#12 (.p12/.pfx)
	ext := strings.ToLower(filepath.Ext(certFile))
	isPKCS12 := ext == ".p12" || ext == ".pfx"

	var cert tls.Certificate
	var err error
	if isPKCS12 {
		cert, err = loadPKCS12(certFile, password)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("failed to load PKCS#12: %w", err)
		}
		slog.Info("Loaded client certificate from PKCS#12", slog.String("p12File", certFile))
	} else {
		// Load PEM
		if keyFile == "" {
			return tls.Certificate{}, fmt.Errorf("key file required when using PEM certificate")
		}
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("failed to load certificate: %w", err)
		}
		slog.Info("Loaded client certificate from PEM", slog.String("certFile", certFile), slog.String("keyFile", keyFile))
	}

	// Make sure leaf is populated
	if len(cert.Certificate) == 0 {
		return tls.Certificate{}, fmt.Errorf("no certificates found in file")
	}
	if cert.Leaf == nil {
		cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("failed to parse leaf certificate: %w", err)
		}
	}

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
	// TLSCAFile is the CA certificate file to verify MITZ server
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
