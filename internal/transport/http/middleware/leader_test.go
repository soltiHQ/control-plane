package middleware

import "testing"

func TestBuildTarget(t *testing.T) {
	tests := []struct {
		name       string
		leaderAddr string
		port       int
		tls        bool
		want       string
	}{
		{"ipv4 host:port → http", "10.0.0.1:7000", 8080, false, "http://10.0.0.1:8080"},
		{"ipv4 with tls → https", "10.0.0.1:7000", 8080, true, "https://10.0.0.1:8080"},
		{"ipv6 bracketed → rebracketed", "[::1]:7000", 8080, false, "http://[::1]:8080"},
		{"bare host (no port)", "leader-host", 8082, false, "http://leader-host:8082"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := buildTarget(tt.leaderAddr, tt.port, tt.tls)
			if err != nil {
				t.Fatalf("buildTarget error: %v", err)
			}
			if got := u.String(); got != tt.want {
				t.Fatalf("buildTarget(%q, %d, %v) = %q, want %q", tt.leaderAddr, tt.port, tt.tls, got, tt.want)
			}
		})
	}
}
