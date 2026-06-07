package response

import (
	"net/http"

	"github.com/soltiHQ/control-plane/internal/transport/errkind"
	"github.com/soltiHQ/control-plane/internal/transport/http/responder"
	"github.com/soltiHQ/control-plane/internal/transport/httpctx"
	"github.com/soltiHQ/control-plane/internal/transportctx"

	"github.com/a-h/templ"
)

// FromError maps a domain error to the matching HTTP response using the shared errkind classifier.
func FromError(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode, err error) {
	switch errkind.Classify(err) {
	case errkind.InvalidArgument:
		BadRequest(w, r, mode)
	case errkind.Unauthenticated:
		Unauthorized(w, r, mode)
	case errkind.PermissionDenied:
		Forbidden(w, r, mode)
	case errkind.NotFound:
		NotFound(w, r, mode)
	case errkind.AlreadyExists:
		Conflict(w, r, mode, "already exists")
	case errkind.Conflict:
		Conflict(w, r, mode, "conflict")
	case errkind.FailedPrecondition:
		Conflict(w, r, mode, "precondition failed")
	case errkind.Unavailable, errkind.Canceled, errkind.DeadlineExceeded:
		Unavailable(w, r, mode)
	default:
		Internal(w, r, mode)
	}
}

type errorBody struct {
	Code      int    `json:"code"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// OK renders a 200 response.
func OK(w http.ResponseWriter, r *http.Request, _ httpctx.RenderMode, v *responder.View) {
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusOK, v)
}

// NoContent renders a 204 response.
func NoContent(w http.ResponseWriter, r *http.Request) {
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusNoContent, nil)
}

// NotFound renders a 404 response.
func NotFound(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	transportctx.SetError(r.Context(), "not found")
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusNotFound, &responder.View{
		Data: errorBody{
			Code:      http.StatusNotFound,
			Message:   "not found",
			RequestID: transportctx.TryRequestID(r.Context()),
		},
		Component: func(m httpctx.RenderMode) templ.Component {
			if m == httpctx.RenderPage {
				return renderErrorPage(
					http.StatusNotFound,
					"Page not found",
					"The page you are looking for doesn't exist or has been moved.",
				)
			}
			return nil
		}(mode),
	})
}

// BadRequest renders a 400 response.
func BadRequest(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	BadRequestMsg(w, r, mode, "invalid request")
}

// BadRequestMsg renders a 400 response with a custom message.
func BadRequestMsg(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode, msg string) {
	transportctx.SetError(r.Context(), msg)
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusBadRequest, &responder.View{
		Data: errorBody{
			Code:      http.StatusBadRequest,
			Message:   msg,
			RequestID: transportctx.TryRequestID(r.Context()),
		},
		Component: func(m httpctx.RenderMode) templ.Component {
			if m == httpctx.RenderPage {
				return renderErrorPage(
					http.StatusBadRequest,
					"Invalid request",
					msg,
				)
			}
			return nil
		}(mode),
	})
}

// Conflict renders a 409 response.
func Conflict(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode, msg string) {
	transportctx.SetError(r.Context(), msg)
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusConflict, &responder.View{
		Data: errorBody{
			Code:      http.StatusConflict,
			Message:   msg,
			RequestID: transportctx.TryRequestID(r.Context()),
		},
		Component: func(m httpctx.RenderMode) templ.Component {
			if m == httpctx.RenderPage {
				return renderErrorPage(
					http.StatusConflict,
					"Conflict",
					msg,
				)
			}
			return nil
		}(mode),
	})
}

// NotAllowed renders a 405 response.
func NotAllowed(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	transportctx.SetError(r.Context(), "not allowed")
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusMethodNotAllowed, &responder.View{
		Data: errorBody{
			Code:      http.StatusMethodNotAllowed,
			Message:   "not allowed",
			RequestID: transportctx.TryRequestID(r.Context()),
		},
		Component: func(m httpctx.RenderMode) templ.Component {
			if m == httpctx.RenderPage {
				return renderErrorPage(
					http.StatusMethodNotAllowed,
					"Method not allowed",
					"The method you used is not allowed on this resource.",
				)
			}
			return nil
		}(mode),
	})
}

// Forbidden renders a 403 response.
func Forbidden(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	transportctx.SetError(r.Context(), "forbidden")
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusForbidden, &responder.View{
		Data: errorBody{
			Code:      http.StatusForbidden,
			Message:   "forbidden",
			RequestID: transportctx.TryRequestID(r.Context()),
		},
		Component: func(m httpctx.RenderMode) templ.Component {
			if m == httpctx.RenderPage {
				return renderErrorPage(
					http.StatusForbidden,
					"Forbidden",
					"You are not allowed to access this page.",
				)
			}
			return nil
		}(mode),
	})
}

// Unauthorized renders a 401 response.
func Unauthorized(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	transportctx.SetError(r.Context(), "unauthorized")
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusUnauthorized, &responder.View{
		Data: errorBody{
			Code:      http.StatusUnauthorized,
			Message:   "unauthorized",
			RequestID: transportctx.TryRequestID(r.Context()),
		},
		Component: func(m httpctx.RenderMode) templ.Component {
			if m == httpctx.RenderPage {
				return renderErrorPage(
					http.StatusUnauthorized,
					"Unauthorized",
					"You are not authorized to access this page.",
				)
			}
			return nil
		}(mode),
	})
}

// Unavailable renders a 503 response.
func Unavailable(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	transportctx.SetError(r.Context(), "service unavailable")
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusServiceUnavailable, &responder.View{
		Data: errorBody{
			Code:      http.StatusServiceUnavailable,
			Message:   "service unavailable",
			RequestID: transportctx.TryRequestID(r.Context()),
		},
		Component: func(m httpctx.RenderMode) templ.Component {
			if m == httpctx.RenderPage {
				return renderErrorPage(
					http.StatusServiceUnavailable,
					"Service unavailable",
					"The server is temporarily unable to handle the request.",
				)
			}
			return nil
		}(mode),
	})
}

// Internal renders a 500 response.
func Internal(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	transportctx.SetError(r.Context(), "internal error")
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusInternalServerError, &responder.View{
		Data: errorBody{
			Code:      http.StatusInternalServerError,
			Message:   "internal error",
			RequestID: transportctx.TryRequestID(r.Context()),
		},
		Component: func(m httpctx.RenderMode) templ.Component {
			if m == httpctx.RenderPage {
				return renderErrorPage(
					http.StatusInternalServerError,
					"Server error",
					"Something went wrong on our side. Please try again later.",
				)
			}
			return nil
		}(mode),
	})
}

// AuthRateLimit renders a 429 response.
func AuthRateLimit(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	transportctx.SetError(r.Context(), "rate limited")
	httpctx.Responder(r.Context()).Respond(w, r, http.StatusTooManyRequests, &responder.View{
		Data: errorBody{
			Code:      http.StatusTooManyRequests,
			Message:   "too many auth requests",
			RequestID: transportctx.TryRequestID(r.Context()),
		},
		Component: func(m httpctx.RenderMode) templ.Component {
			if m == httpctx.RenderPage {
				return renderErrorPage(
					http.StatusTooManyRequests,
					"Too many auth attempts",
					"Account temporarily locked. Please try again later.",
				)
			}
			return nil
		}(mode),
	})
}
