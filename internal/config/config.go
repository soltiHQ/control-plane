// Package config aggregates per-package configuration structs and
// provides Load / Default constructors:
//   - Load reads: defaults → YAML file → ENV overrides (SOLTI_ prefix).
//   - Default returns safe development defaults without external sources.
package config

import (
	"flag"
	"fmt"
	"os"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3"

	"github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/cluster"
	"github.com/soltiHQ/control-plane/internal/server"
	"github.com/soltiHQ/control-plane/internal/server/runner/grpcserver"
	"github.com/soltiHQ/control-plane/internal/server/runner/httpserver"
	"github.com/soltiHQ/control-plane/internal/server/runner/lifecycle"
	syncrunner "github.com/soltiHQ/control-plane/internal/server/runner/sync"
	"github.com/soltiHQ/control-plane/internal/transport/http/middleware"
	"github.com/soltiHQ/control-plane/internal/transport/tlsconf"
	"github.com/soltiHQ/control-plane/internal/uikit/htmx"
)

const (
	envPrefix = "SOLTI"
)

// TLSConfig groups the control-plane's TLS settings: the server identity used by
// the HTTP and gRPC listeners, and the client identity used when dialing agents.
// Both are opt-in — empty means plaintext.
type TLSConfig struct {
	Server tlsconf.ServerConfig `yaml:"server" envconfig:"SERVER"`
	Client tlsconf.ClientConfig `yaml:"client" envconfig:"CLIENT"`
}

// Config holds the full application configuration.
type Config struct {
	HTTP          httpserver.Config        `yaml:"http"           envconfig:"HTTP"`
	HTTPDiscovery httpserver.Config        `yaml:"http_discovery" envconfig:"HTTP_DISCOVERY"`
	GRPC          grpcserver.Config        `yaml:"grpc"           envconfig:"GRPC"`
	Sync          syncrunner.Config        `yaml:"sync"           envconfig:"SYNC"`
	Lifecycle     lifecycle.Config         `yaml:"lifecycle"      envconfig:"LIFECYCLE"`
	Triggers      htmx.Config              `yaml:"triggers"       envconfig:"TRIGGERS"`
	Server        server.Config            `yaml:"server"         envconfig:"SERVER"`
	Auth          auth.Config              `yaml:"auth"           envconfig:"AUTH"`
	CORS          middleware.CORSConfig    `yaml:"cors"           envconfig:"CORS"`
	Cluster       cluster.Config           `yaml:"cluster"        envconfig:"CLUSTER"`
	Streams       middleware.StreamsConfig `yaml:"streams"        envconfig:"STREAMS"`
	TLS           TLSConfig                `yaml:"tls"            envconfig:"TLS"`
}

// Default returns the default development configuration.
//
// Security-sensitive fields (e.g. [auth.Config.JWTSecret]) are intentionally
// left empty here: they must come from the environment (SOLTI_AUTH_JWT_SECRET)
// or YAML at load time. [Load] enforces this via [Config.Validate].
func Default() Config {
	return Config{
		HTTP:          httpserver.Config{Name: "http", Addr: ":8080"},
		HTTPDiscovery: httpserver.Config{Name: "http-discovery", Addr: ":8082"},
		GRPC:          grpcserver.Config{Name: "grpc-discovery", Addr: ":50051"},
		Auth:          auth.Config{}, // JWTSecret must be supplied externally
		CORS: middleware.CORSConfig{
			AllowOrigins: []string{"*"},
		},
		Cluster: cluster.DefaultConfig(),
		Streams: middleware.StreamsConfig{
			MaxPerIP:         10,
			SubscriberBuffer: 64,
		},
	}
}

// Validate reports an error if any sub-config has invalid or unsafe values.
func (c Config) Validate() error {
	if err := c.Auth.Validate(); err != nil {
		return fmt.Errorf("config.auth: %w", err)
	}
	if err := c.CORS.Validate(); err != nil {
		return fmt.Errorf("config.cors: %w", err)
	}
	if err := c.TLS.Server.Validate(); err != nil {
		return fmt.Errorf("config.tls.server: %w", err)
	}
	if err := c.TLS.Client.Validate(); err != nil {
		return fmt.Errorf("config.tls.client: %w", err)
	}
	return nil
}

// Load reads configuration in priority order: defaults → YAML file → ENV.
// Refuses to return a Config that fails [Config.Validate].
func Load() (Config, error) {
	var (
		path = configPath()
		cfg  = Default()
	)
	if path != "" {
		if err := loadYAML(path, &cfg); err != nil {
			return Config{}, fmt.Errorf("config: %w", err)
		}
	}
	if err := envconfig.Process(envPrefix, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: env: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// configPath returns the config file path from --config flag or CONFIG_PATH env.
func configPath() string {
	var path string
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.StringVar(&path, "config", "", "path to YAML config file")
	_ = fs.Parse(os.Args[1:])

	if path != "" {
		return path
	}
	return os.Getenv("CONFIG_PATH")
}

// loadYAML reads and unmarshals a YAML file into dst.
func loadYAML(path string, dst *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if err = yaml.NewDecoder(f).Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
