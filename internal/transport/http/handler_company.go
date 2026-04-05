package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeanprocha/backend-engine-go/internal/auth"
	"github.com/jeanprocha/backend-engine-go/internal/company"
)

// listCompaniesHandler lista todos os templates de empresa do usuário.
// GET /companies
func (s *Server) listCompaniesHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "nao autenticado")
		return
	}
	userID = strings.TrimSpace(userID)

	companies, err := s.companies.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro ao listar empresas: "+err.Error())
		return
	}

	resp := make([]CompanyResponse, 0, len(companies))
	for _, c := range companies {
		resp = append(resp, companyToResponse(c))
	}
	writeJSON(w, http.StatusOK, resp)
}

// createCompanyHandler cria um novo template de empresa.
// POST /companies
func (s *Server) createCompanyHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "nao autenticado")
		return
	}
	userID = strings.TrimSpace(userID)

	var req CompanyCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "payload inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name obrigatório")
		return
	}

	id, err := s.companies.Create(r.Context(), company.Company{
		UserID:          userID,
		Name:            req.Name,
		TaxContext:      req.TaxContext,
		DefaultServices: req.DefaultServices,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro ao criar empresa: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id.String()})
}

// deleteCompanyHandler exclui um template de empresa.
// DELETE /companies/{id}
func (s *Server) deleteCompanyHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := s.companies.Delete(r.Context(), userID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "empresa não encontrada")
			return
		}
		writeError(w, http.StatusInternalServerError, "erro ao excluir empresa: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func companyToResponse(c company.Company) CompanyResponse {
	services := c.DefaultServices
	if len(services) == 0 {
		services = []byte("[]")
	}
	return CompanyResponse{
		ID:              c.ID.String(),
		Name:            c.Name,
		TaxContext:      c.TaxContext,
		DefaultServices: services,
		CreatedAt:       c.CreatedAt.UTC().Format(time.RFC3339),
	}
}
