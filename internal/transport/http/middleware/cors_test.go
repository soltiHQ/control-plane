package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     CORSConfig
		wantErr bool
	}{
		{"wildcard without credentials is ok", CORSConfig{AllowOrigins: []string{"*"}}, false},
		{"wildcard with credentials is rejected", CORSConfig{AllowOrigins: []string{"*"}, AllowCredentials: true}, true},
		{"specific origin with credentials is ok", CORSConfig{AllowOrigins: []string{"https://app.example.com"}, AllowCredentials: true}, false},
		{"no origins is ok", CORSConfig{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func corsHandler(cfg CORSConfig) http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return CORS(cfg)(next)
}

func TestCORSAllowedOriginIsReflected(t *testing.T) {
	h := corsHandler(CORSConfig{AllowOrigins: []string{"https://app.example.com"}})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Allow-Origin = %q, want reflected origin", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
}

func TestCORSWildcardWithoutCredentials(t *testing.T) {
	h := corsHandler(CORSConfig{AllowOrigins: []string{"*"}})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://any.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q, want *", got)
	}
}

func TestCORSDisallowedOriginPreflightForbidden(t *testing.T) {
	h := corsHandler(CORSConfig{AllowOrigins: []string{"https://app.example.com"}})
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("preflight from disallowed origin: code = %d, want 403", w.Code)
	}
}

func TestCORSDisallowedOriginSimpleRequestPassesWithoutHeaders(t *testing.T) {
	h := corsHandler(CORSConfig{AllowOrigins: []string{"https://app.example.com"}})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (handler runs)", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin = %q, want empty for disallowed origin", got)
	}
}

func TestCORSPreflightAllowed(t *testing.T) {
	h := corsHandler(CORSConfig{AllowOrigins: []string{"https://app.example.com"}})
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight code = %d, want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("missing Access-Control-Allow-Methods on preflight")
	}
}

func TestCORSNoOriginPassesThrough(t *testing.T) {
	h := corsHandler(CORSConfig{AllowOrigins: []string{"https://app.example.com"}})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin = %q, want empty (no Origin header)", got)
	}
}
