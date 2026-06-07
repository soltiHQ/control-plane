package httpserver

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

const (
	defaultReadHeaderTimeout = 10 * time.Second
	defaultReadTimeout       = 30 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultMaxHeaderBytes    = 1 << 14 // 16 KB
)

// Config controls HTTP server runtime behavior.
type Config struct {
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes"`

	Name string `yaml:"name"`
	Addr string `yaml:"addr"`

	// TLSConfig, when non-nil, makes the runner serve HTTPS (via ServeTLS).
	// Set by the app wiring from the shared config.TLS; not loaded from YAML.
	TLSConfig *tls.Config `yaml:"-"`

	BaseContext func(net.Listener) context.Context                    `yaml:"-"`
	ConnContext func(ctx context.Context, c net.Conn) context.Context `yaml:"-"`
}

func (c Config) withDefaults() Config {
	if c.Name == "" {
		c.Name = "http"
	}
	if c.ReadHeaderTimeout <= 0 {
		c.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = defaultReadTimeout
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = defaultWriteTimeout
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.MaxHeaderBytes <= 0 {
		c.MaxHeaderBytes = defaultMaxHeaderBytes
	}
	return c
}
