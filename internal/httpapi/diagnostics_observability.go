package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/observability"
)

type diagnosticsObserver interface {
	Begin(context.Context, observability.Operation) (context.Context, func(observability.Outcome))
}

// SetDiagnosticsObserver borrows an adapter before Start. Its composition-root
// owner, not the HTTP server, owns provider shutdown. Nil preserves existing behavior.
func (s *Server) SetDiagnosticsObserver(observer diagnosticsObserver) {
	s.diagnostics = observer
}

func diagnosticOperation(method, path string) observability.Operation {
	path = strings.TrimSuffix(path, "/")
	if path == "/api/diagnostics/ui-performance" {
		switch method {
		case http.MethodGet:
			return observability.UITimelineRead
		case http.MethodPost:
			return observability.UITimelineWrite
		}
	}
	if method == http.MethodPost {
		switch path {
		case "/api/diagnostics/instagram-reader":
			return observability.InstagramReader
		case "/api/diagnostics/instagram-bootstrap":
			return observability.InstagramBootstrap
		case "/api/diagnostics/instagram-bootstrap-corroboration":
			return observability.InstagramCorroboration
		}
	}
	return 0
}

func (s *Server) serveObservedRoute(w http.ResponseWriter, r *http.Request) {
	operation := diagnosticOperation(r.Method, r.URL.Path)
	if s.diagnostics == nil || operation == 0 {
		if err := s.route(w, r); err != nil {
			s.writeError(w, err)
		}
		return
	}
	ctx, finish := s.diagnostics.Begin(r.Context(), operation)
	response := &diagnosticResponseWriter{ResponseWriter: w}
	completed := false
	defer func() {
		outcome := observability.Unavailable
		if completed {
			switch {
			case response.status >= http.StatusInternalServerError:
			case response.status >= http.StatusBadRequest:
				outcome = observability.Rejected
			default:
				outcome = observability.Success
			}
		}
		finish(outcome)
	}()
	if err := s.route(response, r.WithContext(ctx)); err != nil {
		s.writeError(response, err)
	}
	completed = true
}

// Only the allowlisted JSON diagnostic routes are wrapped. Streaming, native
// capabilities and unrelated routes retain their original ResponseWriter.
type diagnosticResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *diagnosticResponseWriter) WriteHeader(status int) {
	if w.status == 0 && (status >= 200 || status == http.StatusSwitchingProtocols) {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *diagnosticResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (w *diagnosticResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
