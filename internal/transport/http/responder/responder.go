package responder

import (
	"net/http"

	"github.com/a-h/templ"
)

// Responder writes HTTP responses in a format appropriate for the client.
type Responder interface {
	Respond(w http.ResponseWriter, r *http.Request, code int, v *View)
}

// View carries response data in a format-agnostic way.
type View struct {
	// Component is a templ component for HTML rendering.
	// Ignored by JSONResponder.
	Component templ.Component
	// Data is the response payload for JSON rendering, marshalled via encoding/json.
	// Ignored by HTMLResponder and when RawJSON is set.
	Data any
	// RawJSON is an already-marshalled JSON body written verbatim by JSONResponder. U
	//
	// Use for proto-JSON (canonical camelCase) or any format where encoding/json would corrupt the output.
	// Ignored by HTMLResponder.
	// Takes precedence over Data.
	RawJSON []byte
}
