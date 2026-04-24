package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// healthHandler: liveness para orquestradores (Railway, K8s) — resposta HTTP 200
// se o processo recebe pedidos, mesmo com base indisponível. O corpo indica
// o estado de readiness da DB; monitore `status` / `db` fora do orquestrador.
// Ver também GET /ready (falha com 503 se a DB não responder).
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

	writeJSON(w, http.StatusOK, HealthResponse{Status: status, DB: dbStatus})
}

// readyHandler: readiness (DB). Railway pode usar /health (liveness) e /ready (dependências).
// GET /ready → 200 se a DB responde, 503 caso contrário.
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, HealthResponse{Status: "unready", DB: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", DB: "ok"})
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
