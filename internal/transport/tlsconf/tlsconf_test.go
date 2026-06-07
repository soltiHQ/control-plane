package tlsconf

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSelfSigned(t *testing.T) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	writePEM(t, certPath, "CERTIFICATE", der)
	writePEM(t, keyPath, "PRIVATE KEY", keyDER)
	return certPath, keyPath
}

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: typ, Bytes: der}); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}

func TestServerConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ServerConfig
		wantErr bool
	}{
		{"empty is ok (plaintext)", ServerConfig{}, false},
		{"cert+key ok", ServerConfig{CertFile: "c", KeyFile: "k"}, false},
		{"cert without key", ServerConfig{CertFile: "c"}, true},
		{"key without cert", ServerConfig{KeyFile: "k"}, true},
		{"client CA without cert", ServerConfig{ClientCAFile: "ca"}, true},
		{"client CA with cert+key ok", ServerConfig{CertFile: "c", KeyFile: "k", ClientCAFile: "ca"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerConfigBuildDisabled(t *testing.T) {
	cfg, err := ServerConfig{}.Build()
	if err != nil || cfg != nil {
		t.Fatalf("disabled Build() = (%v, %v), want (nil, nil)", cfg, err)
	}
}

func TestServerConfigBuild(t *testing.T) {
	cert, key := writeSelfSigned(t)
	cfg, err := ServerConfig{CertFile: cert, KeyFile: key}.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates = %d, want 1", len(cfg.Certificates))
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want TLS1.2", cfg.MinVersion)
	}
	if cfg.ClientAuth != tls.NoClientCert {
		t.Fatalf("ClientAuth = %v, want NoClientCert (no mTLS)", cfg.ClientAuth)
	}
}

func TestServerConfigBuildMTLS(t *testing.T) {
	cert, key := writeSelfSigned(t)
	cfg, err := ServerConfig{CertFile: cert, KeyFile: key, ClientCAFile: cert}.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("ClientAuth = %v, want RequireAndVerifyClientCert", cfg.ClientAuth)
	}
	if cfg.ClientCAs == nil {
		t.Fatalf("ClientCAs is nil, want pool")
	}
}

func TestClientConfigBuild(t *testing.T) {
	cert, key := writeSelfSigned(t)

	t.Run("disabled", func(t *testing.T) {
		cfg, err := ClientConfig{}.Build()
		if err != nil || cfg != nil {
			t.Fatalf("disabled Build() = (%v, %v), want (nil, nil)", cfg, err)
		}
	})

	t.Run("ca only", func(t *testing.T) {
		cfg, err := ClientConfig{CAFile: cert}.Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if cfg.RootCAs == nil {
			t.Fatalf("RootCAs is nil, want pool")
		}
		if len(cfg.Certificates) != 0 {
			t.Fatalf("Certificates = %d, want 0 (no client cert)", len(cfg.Certificates))
		}
	})

	t.Run("mtls client cert", func(t *testing.T) {
		cfg, err := ClientConfig{CAFile: cert, CertFile: cert, KeyFile: key}.Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if len(cfg.Certificates) != 1 {
			t.Fatalf("Certificates = %d, want 1", len(cfg.Certificates))
		}
	})
}

func TestClientConfigValidate(t *testing.T) {
	if err := (ClientConfig{CertFile: "c"}).Validate(); err == nil {
		t.Fatalf("cert without key should error")
	}
	if err := (ClientConfig{CAFile: "ca"}).Validate(); err != nil {
		t.Fatalf("ca-only should be valid, got %v", err)
	}
}
