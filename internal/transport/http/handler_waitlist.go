package http

import (
	"net/http"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/waitlist"
)

// waitlistHandler grava o e-mail do formulário da landing (Etapa M/PR 9) —
// rota pública, sem protectRoute, igual a /credit-classifications/batch.
func (s *Server) waitlistHandler(w http.ResponseWriter, r *http.Request) {
	var req WaitlistJoinRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		writeError(w, http.StatusBadRequest, "campo 'email' não pode ser vazio")
		return
	}
	if err := waitlist.ValidateForInsert(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, "email inválido")
		return
	}

	joined, err := s.waitlist.Join(r.Context(), req.Email)
	if err != nil {
		writeInternalError(w, r, "waitlist_join", err)
		return
	}

	writeJSON(w, http.StatusCreated, WaitlistJoinResponse{Joined: joined})
}
