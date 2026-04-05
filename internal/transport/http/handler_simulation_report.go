package http

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jeanprocha/backend-engine-go/internal/auth"
)

// simulationRecordReportHandler gera PDF de diagnóstico para uma simulação do utilizador.
// GET /simulation-records/{id}/report
func (s *Server) simulationRecordReportHandler(w http.ResponseWriter, r *http.Request) {
	if s.generateDiagnosticPDF == nil {
		writeError(w, http.StatusServiceUnavailable, "geração de relatório indisponível")
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "nao autenticado")
		return
	}
	userID = strings.TrimSpace(userID)

	rawID := strings.TrimSpace(r.PathValue("id"))
	id, err := uuid.Parse(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}

	detail, err := s.history.GetByID(r.Context(), userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro ao carregar simulação")
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "simulação não encontrada")
		return
	}

	pdfBytes, err := s.generateDiagnosticPDF(detail)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro ao gerar PDF")
		return
	}

	filename := fmt.Sprintf("diagnostico-tribuia-%s.pdf", id.String()[:8])
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}
