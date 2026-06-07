package response

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/transport/httpctx"
)

func TestFromError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not found → 404", storage.ErrNotFound, http.StatusNotFound},
		{"already exists → 409", storage.ErrAlreadyExists, http.StatusConflict},
		{"conflict → 409", storage.ErrConflict, http.StatusConflict},
		{"invalid argument → 400", storage.ErrInvalidArgument, http.StatusBadRequest},
		{"unauthorized → 403", auth.ErrUnauthorized, http.StatusForbidden},
		{"invalid token → 401", auth.ErrInvalidToken, http.StatusUnauthorized},
		{"user disabled → 409", auth.ErrUserDisabled, http.StatusConflict},
		{"canceled → 503", context.Canceled, http.StatusServiceUnavailable},
		{"unknown → 500", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()

			FromError(w, r, httpctx.RenderBlock, tt.err)

			if w.Code != tt.want {
				t.Fatalf("FromError(%v) status = %d, want %d", tt.err, w.Code, tt.want)
			}
		})
	}
}
