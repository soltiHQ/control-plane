package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CORSConfig controls CORS behavior.
type CORSConfig struct {
	// AllowOrigins is the list of allowed origins.
	// Use ["*"] to allow all (not recommended with credentials).
	AllowOrigins []string `yaml:"allow_origins"`
	// AllowMethods is the list of allowed HTTP methods.
	// Defaults to GET, POST, PUT, DELETE, PATCH, OPTIONS.
	AllowMethods []string `yaml:"allow_methods"`
	// AllowHeaders is the list of allowed request headers.
	// Defaults to Authorization, Content-Type, X-Request-Id.
	AllowHeaders []string `yaml:"allow_headers"`
	// ExposeHeaders is the list of headers the browser can access.
	// Defaults to X-Request-Id.
	ExposeHeaders []string `yaml:"expose_headers"`
	// AllowCredentials indicates whether cookies/auth headers are allowed.
	AllowCredentials bool `yaml:"allow_credentials"`
	// MaxAge is how long the browser caches preflight results.
	// Defaults to 12 hours.
	MaxAge time.Duration `yaml:"max_age"`
}

// Validate rejects unsafe CORS combinations.
//
// Wildcard origin ("*") with AllowCredentials is forbidden.
func (c CORSConfig) Validate() error {
	if !c.AllowCredentials {
		return nil
	}
	for _, o := range c.AllowOrigins {
		if o == "*" {
			return errors.New(`allow_credentials cannot be combined with wildcard origin "*"`)
		}
	}
	return nil
}

func (c CORSConfig) withDefaults() CORSConfig {
	if len(c.AllowMethods) == 0 {
		c.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}
	}
	if len(c.AllowHeaders) == 0 {
		c.AllowHeaders = []string{"Authorization", "Content-Type", "X-Request-Id"}
	}
	if len(c.ExposeHeaders) == 0 {
		c.ExposeHeaders = []string{"X-Request-Id"}
	}
	if c.MaxAge <= 0 {
		c.MaxAge = 12 * time.Hour
	}
	return c
}

// CORS returns middleware that handles Cross-Origin Resource Sharing.
//
// Preflight (OPTIONS) requests are answered immediately.
// Actual requests get CORS headers attached.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	cfg = cfg.withDefaults()

	var (
		maxAge   = strconv.FormatInt(int64(cfg.MaxAge.Seconds()), 10)
		origins  = make(map[string]struct{}, len(cfg.AllowOrigins))
		allowAll = false

		allowMethods  = strings.Join(cfg.AllowMethods, ", ")
		allowHeaders  = strings.Join(cfg.AllowHeaders, ", ")
		exposeHeaders = strings.Join(cfg.ExposeHeaders, ", ")
	)
	for _, o := range cfg.AllowOrigins {
		if o == "*" {
			allowAll = true
			continue
		}
		origins[o] = struct{}{}
	}
	isAllowedOrigin := func(origin string) bool {
		if origin == "" {
			return false
		}
		if allowAll {
			return true
		}
		_, ok := origins[origin]
		return ok
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var (
				origin = r.Header.Get("Origin")
				h      = w.Header()
			)
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if !isAllowedOrigin(origin) {
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if allowAll && !cfg.AllowCredentials {
				h.Set("Access-Control-Allow-Origin", "*")
			} else {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
			}
			if cfg.AllowCredentials {
				h.Set("Access-Control-Allow-Credentials", "true")
			}
			if exposeHeaders != "" {
				h.Set("Access-Control-Expose-Headers", exposeHeaders)
			}
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", allowMethods)
				h.Set("Access-Control-Allow-Headers", allowHeaders)
				h.Set("Access-Control-Max-Age", maxAge)

				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")

				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
