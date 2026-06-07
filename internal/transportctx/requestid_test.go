package transportctx

import (
	"strings"
	"testing"
)

func TestNormalizeRequestID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"valid ksuid", "2cV8nT0aBcD1eFgHiJkLmNoPqRs", "2cV8nT0aBcD1eFgHiJkLmNoPqRs"},
		{"safe punctuation", "req-id_1.2", "req-id_1.2"},
		{"trims surrounding space", "  abc123  ", "abc123"},
		{"empty", "", ""},
		{"only whitespace", "   ", ""},
		{"inner space rejected", "abc def", ""},
		{"newline rejected (log injection)", "abc\ndef", ""},
		{"carriage return rejected", "abc\rdef", ""},
		{"null byte rejected", "abc\x00def", ""},
		{"slash rejected", "abc/def", ""},
		{"too long rejected", strings.Repeat("a", maxRequestIDLen+1), ""},
		{"max length accepted", strings.Repeat("a", maxRequestIDLen), strings.Repeat("a", maxRequestIDLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeRequestID(tt.in); got != tt.want {
				t.Fatalf("NormalizeRequestID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewRequestIDIsNormalizable(t *testing.T) {
	id := NewRequestID()
	if got := NormalizeRequestID(id); got != id {
		t.Fatalf("generated id %q failed normalization: got %q", id, got)
	}
}
