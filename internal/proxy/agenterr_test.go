package proxy

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/soltiHQ/control-plane/internal/transport/errkind"
)

type httpStatusErr struct{ code int }

func (e httpStatusErr) Error() string   { return "http error" }
func (e httpStatusErr) HTTPStatus() int { return e.code }

func TestClassifyAgentErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want errkind.Kind
	}{
		{"grpc not found", grpcstatus.Error(codes.NotFound, "x"), errkind.NotFound},
		{"grpc invalid arg", grpcstatus.Error(codes.InvalidArgument, "x"), errkind.InvalidArgument},
		{"grpc unavailable", grpcstatus.Error(codes.Unavailable, "x"), errkind.Unavailable},
		{"http 404", httpStatusErr{code: 404}, errkind.NotFound},
		{"http 409", httpStatusErr{code: 409}, errkind.Conflict},
		{"cp-side (no status)", errors.New("dial failed"), errkind.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyAgentErr(tt.err); got != tt.want {
				t.Fatalf("classifyAgentErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestAgentErrorCarriesKindAndSentinel(t *testing.T) {
	inner := grpcstatus.Error(codes.NotFound, "task gone")
	err := agentError(ErrGetTask, inner)

	// errkind reads the tagged Kind through the single contract.
	if got := errkind.Classify(err); got != errkind.NotFound {
		t.Fatalf("Classify = %v, want NotFound", got)
	}
	// The op sentinel stays in the chain for errors.Is.
	if !errors.Is(err, ErrGetTask) {
		t.Fatalf("errors.Is(ErrGetTask) = false, want true")
	}
}
