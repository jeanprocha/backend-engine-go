package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jeanprocha/backend-engine-go/internal/reqctx"
)

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		ctx := reqctx.WithID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestIDFromContext é o único ponto deste pacote que lê o request_id —
// delega a internal/reqctx, que também é importado por internal/classifier
// para correlacionar a re-tentativa de parse com o restante dos logs da
// mesma requisição (ver credit_classification_parse_retry em service.go).
func requestIDFromContext(ctx context.Context) string {
	return reqctx.FromContext(ctx)
}
