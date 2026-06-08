package handler

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/ui"
)

// Static serves static files from the UI package.
type Static struct {
	server http.Handler
}

// NewStatic creates a new Static handler.
func NewStatic(logger zerolog.Logger) *Static {
	sub, err := fs.Sub(ui.Static, "static")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load static files")
	}
	return &Static{
		server: http.FileServer(http.FS(sub)),
	}
}

// Routes register the handler routes.
func (s *Static) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/static", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/", http.StatusMovedPermanently)
	})

	mux.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(s.serve)))
}

func (s *Static) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/") {
		http.NotFound(w, r)
		return
	}
	// Prevent browsers from MIME-sniffing served assets into a different
	// content type — cheap defense-in-depth, always safe for static files.
	w.Header().Set("X-Content-Type-Options", "nosniff")

	s.server.ServeHTTP(w, r)
}
