package http

import (
	"crypto/subtle"
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"
)

// maxRequestBodyBytes bounds the body of every request handled by central.
// Oversized bodies fail the same way a malformed body does: the JSON decoder
// in the handler surfaces a decode error and the handler returns its existing
// validation response.
const maxRequestBodyBytes = 1 << 20

// statusRecorder captures the response status so a recovery middleware can
// avoid writing a second, conflicting status line after a handler panicked
// mid-response.
type statusRecorder struct {
	http.ResponseWriter
	wroteHeader bool
	status      int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// Recovery converts a handler panic into a 500 response and logs the stack
// trace. A single bad request can never crash the server.
func Recovery(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w}
			defer func() {
				if v := recover(); v != nil {
					log.Error("panic recovered",
						zap.Any("panic", v),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
						zap.ByteString("stack", debug.Stack()),
					)
					if !rec.wroteHeader {
						writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
					}
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// MaxBodyBytes caps the request body with http.MaxBytesReader so a client
// cannot exhaust server memory. The error surfaces as the handler's existing
// invalid-request validation error.
func MaxBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// OperatorAuth is the middleware boundary for operator-facing endpoints. It
// accepts a bearer token configured on central and performs a constant-time
// comparison. RBAC and finer operator identity are intentionally deferred; this
// only establishes the boundary.
func OperatorAuth(token string) func(http.Handler) http.Handler {
	expected := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := bearerToken(r)
			if token == "" || len(got) == 0 || subtle.ConstantTimeCompare(expected, got) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized", "invalid operator credentials")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) []byte {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if len(auth) < len(prefix) {
		return nil
	}
	if auth[:len(prefix)] != prefix {
		return nil
	}
	return []byte(auth[len(prefix):])
}
