package http

import (
	"log/slog"
	"net/http"
)

const msgInternalError = "erro interno; tente novamente"

// writeInternalError regista o erro com request_id e path; resposta JSON genérica ao cliente.
func writeInternalError(w http.ResponseWriter, r *http.Request, logKey string, err error) {
	rid := requestIDFromContext(r.Context())
	slog.Error("internal_error",
		"log_key", logKey,
		"err", err,
		"request_id", rid,
		"path", r.URL.Path,
		"method", r.Method,
	)
	writeJSON(w, http.StatusInternalServerError, ErrorResponse{
		Error:     msgInternalError,
		Code:      "internal_error",
		RequestID: rid,
	})
}
