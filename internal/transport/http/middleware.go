package http

import (
	"log/slog"
	"net/http"
	"os"
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
		rid := requestIDFromContext(r.Context())
		slog.Info("http_request",
			"request_id", rid,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// corsAllowOrigin devolve o valor de Access-Control-Allow-Origin ou vazio se não permitido.
// CORS_ALLOWED_ORIGINS: lista separada por vírgulas (ex.: origem exata
// "https://tribia.vercel.app" ou preview "https://tribia-xxx.vercel.app" — cada
// origem de preview requer uma entrada, ou defina múltiplas separadas por vírgula).
// Vazia + ENV≠production → "*".
// Vazia + production → sem header (navegador bloqueia cross-origin): defina
// CORS_ALLOWED_ORIGINS no Railway antes do go-live.
func corsAllowOrigin(r *http.Request) string {
	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	isProd := strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production")

	if raw == "" {
		if isProd {
			if origin != "" {
				slog.Warn("cors_production_no_allowlist", "origin", origin)
			}
			return ""
		}
		return "*"
	}
	if origin == "" {
		return ""
	}
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(strings.TrimSuffix(o, "/"))
		if o == "" {
			continue
		}
		oo := strings.TrimSuffix(origin, "/")
		if o == oo {
			return origin
		}
	}
	return ""
}

// withCORS define origem via CORS_ALLOWED_ORIGINS; preflight em OPTIONS; Vary: Origin
// em listas fechadas para caches correctos (Vercel + CDN).
func withCORS(next http.Handler) http.Handler {
	allowHeaders := "Content-Type, Authorization, X-User-ID, X-Tribia-Plan"
	allowMethods := "GET, POST, OPTIONS, DELETE"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ao := corsAllowOrigin(r)
		if ao != "" {
			w.Header().Set("Access-Control-Allow-Origin", ao)
			if ao != "*" {
				w.Header().Set("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", allowMethods)
		w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
		w.Header().Set("Access-Control-Max-Age", "86400")

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
