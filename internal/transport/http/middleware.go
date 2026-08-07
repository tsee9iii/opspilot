package http

import (
	"context"
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

// Unwrap lets http.NewResponseController reach the underlying ResponseWriter so
// SSE streaming can clear the global write deadline and flush mid-response.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
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

// actorContextKey carries the authenticated operator actor through the request
// context. It is only set after bearer authentication has succeeded; a request
// without an actor header never reaches a handler that requires one.
type actorContextKey struct{}

// OperatorActor returns the actor identified by the X-Operator-Actor header,
// or the empty string when no actor was captured.
func OperatorActor(r *http.Request) string {
	v, _ := r.Context().Value(actorContextKey{}).(string)
	return v
}

// maxActorLen bounds the length of an operator actor identifier. The header is
// opaque (a username, an email, or an integration identity) but always bounded.
const maxActorLen = 128

// ActorIdentity is the middleware boundary that records which operator
// performed an action. It runs after OperatorAuth and never authorizes on its
// own: the actor header is treated purely as audit metadata. Endpoints that
// need an actor reject requests that omit or malform it.
func ActorIdentity() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := r.Header.Get("X-Operator-Actor")
			if actor == "" {
				writeError(w, http.StatusBadRequest, "actor_required", "x-operator-actor header is required")
				return
			}
			if len(actor) > maxActorLen {
				writeError(w, http.StatusBadRequest, "actor_required", "x-operator-actor header is too long")
				return
			}
			for _, rn := range actor {
				if rn < 0x20 || rn == 0x7f {
					writeError(w, http.StatusBadRequest, "actor_required", "x-operator-actor header contains invalid characters")
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorContextKey{}, actor)))
		})
	}
}
