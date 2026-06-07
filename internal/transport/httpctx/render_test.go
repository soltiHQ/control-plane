package httpctx

import (
	"net/http"
	"testing"
)

func TestModeFromRequest(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   RenderMode
	}{
		{"htmx request → block", "true", RenderBlock},
		{"non-htmx (header absent) → page", "", RenderPage},
		{"wrong value → page", "false", RenderPage},
		{"case-sensitive: True → page", "True", RenderPage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set(hxRequestHeader, tt.header)
			}
			if got := ModeFromRequest(r); got != tt.want {
				t.Fatalf("ModeFromRequest(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestModeFromRequestNil(t *testing.T) {
	if got := ModeFromRequest(nil); got != RenderPage {
		t.Fatalf("ModeFromRequest(nil) = %v, want RenderPage", got)
	}
}
