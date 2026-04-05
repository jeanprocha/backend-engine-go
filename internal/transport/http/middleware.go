package http

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jeanprocha/backend-engine-go/internal/auth"
)

// ProtectWithClerk valida Authorization: Bearer (JWT Clerk) e injeta o sub no contexto.
func ProtectWithClerk(v *auth.ClerkVerifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, err := v.UserIDFromBearer(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "nao autenticado")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.ContextWithUserID(r.Context(), uid)))
	})
}

// ProtectWithDevHeader (AUTH_SKIP) aceita X-User-ID para desenvolvimento local sem JWKS.
func ProtectWithDevHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := strings.TrimSpace(r.Header.Get("X-User-ID"))
		if uid == "" {
			writeError(w, http.StatusUnauthorized, "X-User-ID obrigatorio (modo AUTH_SKIP)")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.ContextWithUserID(r.Context(), uid)))
	})
}

func protectRoute(devSkip bool, v *auth.ClerkVerifier, next http.Handler) http.Handler {
	if devSkip {
		return ProtectWithDevHeader(next)
	}
	return ProtectWithClerk(v, next)
}

// responseWriter captura o status code para o logger.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// withLogger registra método, path, status e latência de cada request.
func withLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start))
	})
}

// withCORS adiciona os headers de CORS necessários para o frontend Next.js.
// Em produção, substitua o "*" pela origem real.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// chain aplica os middlewares da direita para a esquerda (o primeiro é o mais externo).
func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
