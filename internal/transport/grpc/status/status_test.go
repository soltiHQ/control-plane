package status

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/storage"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"context canceled", context.Canceled, codes.Canceled},
		{"deadline exceeded", context.DeadlineExceeded, codes.DeadlineExceeded},
		{"invalid token → unauthenticated", auth.ErrInvalidToken, codes.Unauthenticated},
		{"revoked → unauthenticated", auth.ErrRevoked, codes.Unauthenticated},
		{"unauthorized → permission denied", auth.ErrUnauthorized, codes.PermissionDenied},
		{"auth invalid argument", auth.ErrInvalidArgument, codes.InvalidArgument},
		{"user disabled → failed precondition", auth.ErrUserDisabled, codes.FailedPrecondition},
		{"not found", storage.ErrNotFound, codes.NotFound},
		{"already exists", storage.ErrAlreadyExists, codes.AlreadyExists},
		{"conflict → aborted", storage.ErrConflict, codes.Aborted},
		{"storage invalid argument", storage.ErrInvalidArgument, codes.InvalidArgument},
		{"unknown → internal", errors.New("boom"), codes.Internal},
		{"wrapped sentinel is unwrapped", fmt.Errorf("load: %w", storage.ErrNotFound), codes.NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, msg := mapError(tt.err)
			if got != tt.want {
				t.Fatalf("mapError(%v) code = %v, want %v", tt.err, got, tt.want)
			}
			if msg == "" {
				t.Fatalf("mapError(%v) returned empty message", tt.err)
			}
		})
	}
}

func TestFromErrorNil(t *testing.T) {
	st := FromError(context.Background(), nil)
	if st.Code() != codes.OK {
		t.Fatalf("FromError(nil) code = %v, want OK", st.Code())
	}
}
