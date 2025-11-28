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

// Config holds TLS configuration options
type Config struct {
	// CertFile is the path to a PEM certificate file OR .p12/.pfx file
	CertFile string
	// KeyFile is the path to a PEM key file (not used if CertFile is .p12/.pfx)
	KeyFile string
	// Password is the password for encrypted key or .p12/.pfx file
	Password string
	// CAFile is the path to a CA certificate file to verify server
	CAFile string
}

// LoadClientCertificate loads a client certificate from PEM or PKCS#12 file
func LoadClientCertificate(certFile, keyFile, password string) (tls.Certificate, error) {
	if certFile == "" {
		return tls.Certificate{}, fmt.Errorf("certificate file not specified")
	}

	// Check if it's PKCS#12 (.p12/.pfx)
	ext := strings.ToLower(filepath.Ext(certFile))
	isPKCS12 := ext == ".p12" || ext == ".pfx"

	if isPKCS12 {
		cert, err := loadPKCS12(certFile, password)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("failed to load PKCS#12: %w", err)
		}
		slog.Info("Loaded client certificate from PKCS#12", "p12File", certFile)
		return cert, nil
	}

	// Load PEM
	if keyFile == "" {
		return tls.Certificate{}, fmt.Errorf("key file required when using PEM certificate")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load certificate: %w", err)
	}
	slog.Info("Loaded client certificate from PEM", "certFile", certFile, "keyFile", keyFile)
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

	slog.Info("Loaded CA certificate", "caFile", caFile)
	return caCertPool, nil
}

// CreateTLSConfig creates a TLS configuration with client certificate and optional CA
func CreateTLSConfig(certFile, keyFile, password, caFile string) (*tls.Config, error) {
	cert, err := LoadClientCertificate(certFile, keyFile, password)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// Load CA certificate if specified
	if caFile != "" {
		caCertPool, err := LoadCACertPool(caFile)
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
