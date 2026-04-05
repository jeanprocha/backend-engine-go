package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// healthHandler retorna o status do servidor e da conexão com o banco.
// GET /health → {"status":"ok","db":"ok"} ou {"status":"degraded","db":"<erro>"}
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	dbStatus := "ok"
	if err := s.store.Ping(ctx); err != nil {
		dbStatus = err.Error()
	}

	status := "ok"
	if dbStatus != "ok" {
		status = "degraded"
	}

	code := http.StatusOK
	if status != "ok" {
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, HealthResponse{Status: status, DB: dbStatus})
}

// writeJSON serializa v como JSON e escreve no ResponseWriter com o status code dado.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError escreve um ErrorResponse JSON com o status code dado.
func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, ErrorResponse{Error: msg})
}
