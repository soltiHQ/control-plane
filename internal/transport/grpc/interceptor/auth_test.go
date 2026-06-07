package interceptor

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestExtractBearerFromMetadata(t *testing.T) {
	tests := []struct {
		name string
		md   metadata.MD
		want string
	}{
		{"standard bearer", metadata.Pairs("authorization", "Bearer tok123"), "tok123"},
		{"lowercase scheme", metadata.Pairs("authorization", "bearer tok123"), "tok123"},
		{"mixed case scheme", metadata.Pairs("authorization", "BeArEr tok123"), "tok123"},
		{"extra spaces trimmed", metadata.Pairs("authorization", "Bearer    tok123  "), "tok123"},
		{"wrong scheme", metadata.Pairs("authorization", "Basic abc"), ""},
		{"scheme only, no token", metadata.Pairs("authorization", "Bearer"), ""},
		{"empty header value", metadata.Pairs("authorization", ""), ""},
		{"no authorization key", metadata.Pairs("x-other", "v"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.md != nil {
				ctx = metadata.NewIncomingContext(ctx, tt.md)
			}
			if got := extractBearerFromMetadata(ctx); got != tt.want {
				t.Fatalf("extractBearerFromMetadata = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractBearerNoMetadata(t *testing.T) {
	if got := extractBearerFromMetadata(context.Background()); got != "" {
		t.Fatalf("extractBearerFromMetadata(no md) = %q, want empty", got)
	}
}
