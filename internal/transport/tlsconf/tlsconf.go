package tlsconf

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

// ServerConfig is the server-side TLS configuration (PEM file paths).
type ServerConfig struct {
	// CertFile is the PEM server certificate chain (leaf first).
	CertFile string `yaml:"cert_file" envconfig:"CERT_FILE"`
	// KeyFile is the PEM private key for CertFile.
	KeyFile string `yaml:"key_file" envconfig:"KEY_FILE"`
	// ClientCAFile, when set, turns on mTLS: clients must present a certificate signed by this CA bundle (tls.RequireAndVerifyClientCert).
	ClientCAFile string `yaml:"client_ca_file" envconfig:"CLIENT_CA_FILE"`
}

// Enabled reports whether server TLS is configured.
func (c ServerConfig) Enabled() bool { return c.CertFile != "" }

// Validate checks for structurally invalid combinations.
func (c ServerConfig) Validate() error {
	if (c.CertFile == "") != (c.KeyFile == "") {
		return errors.New("tls server: cert_file and key_file must be set together")
	}
	if c.ClientCAFile != "" && c.CertFile == "" {
		return errors.New("tls server: client_ca_file requires cert_file and key_file")
	}
	return nil
}

// Build returns the server *tls.Config, or (nil, nil) when TLS is disabled.
func (c ServerConfig) Build() (*tls.Config, error) {
	if !c.Enabled() {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("tls server: load keypair: %w", err)
	}
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}
	if c.ClientCAFile != "" {
		pool, err := loadCAPool(c.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("tls server: client CA: %w", err)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}

// ClientConfig is the client-side TLS configuration (PEM file paths) used by the CP when dialing agents.
type ClientConfig struct {
	CAFile   string `yaml:"ca_file"   envconfig:"CA_FILE"`
	CertFile string `yaml:"cert_file" envconfig:"CERT_FILE"`
	KeyFile  string `yaml:"key_file"  envconfig:"KEY_FILE"`
}

// Enabled reports whether the CP should dial agents over TLS.
func (c ClientConfig) Enabled() bool { return c.CAFile != "" || c.CertFile != "" }

// Validate checks for structurally invalid combinations.
func (c ClientConfig) Validate() error {
	if (c.CertFile == "") != (c.KeyFile == "") {
		return errors.New("tls client: cert_file and key_file must be set together")
	}
	return nil
}

// Build returns the client *tls.Config, or (nil, nil) when TLS is disabled.
func (c ClientConfig) Build() (*tls.Config, error) {
	if !c.Enabled() {
		return nil, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CAFile != "" {
		pool, err := loadCAPool(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("tls client: CA: %w", err)
		}
		cfg.RootCAs = pool
	}
	if c.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("tls client: load keypair: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

// loadCAPool reads a PEM CA bundle into a *x509.CertPool.
func loadCAPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates found in %s", path)
	}
	return pool, nil
}
