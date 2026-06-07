package errkind

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/storage"
)

type kindErr struct{ k Kind }

func (e kindErr) Error() string   { return "tagged" }
func (e kindErr) ErrorKind() Kind { return e.k }

func TestClassifyKinder(t *testing.T) {
	if got := Classify(kindErr{k: NotFound}); got != NotFound {
		t.Fatalf("Classify(kinder NotFound) = %v, want NotFound", got)
	}
	if got := Classify(fmt.Errorf("op: %w", kindErr{k: Conflict})); got != Conflict {
		t.Fatalf("Classify(wrapped kinder Conflict) = %v, want Conflict", got)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Kind
	}{
		{"nil → internal", nil, Internal},
		{"unknown → internal", errors.New("boom"), Internal},
		{"context canceled", context.Canceled, Canceled},
		{"deadline exceeded", context.DeadlineExceeded, DeadlineExceeded},
		{"invalid token → unauthenticated", auth.ErrInvalidToken, Unauthenticated},
		{"revoked → unauthenticated", auth.ErrRevoked, Unauthenticated},
		{"unauthorized → permission denied", auth.ErrUnauthorized, PermissionDenied},
		{"auth invalid argument", auth.ErrInvalidArgument, InvalidArgument},
		{"user disabled → failed precondition", auth.ErrUserDisabled, FailedPrecondition},
		{"not found", storage.ErrNotFound, NotFound},
		{"already exists", storage.ErrAlreadyExists, AlreadyExists},
		{"conflict", storage.ErrConflict, Conflict},
		{"storage invalid argument", storage.ErrInvalidArgument, InvalidArgument},
		{"wrapped sentinel is unwrapped", fmt.Errorf("load: %w", storage.ErrNotFound), NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.err); got != tt.want {
				t.Fatalf("Classify(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
