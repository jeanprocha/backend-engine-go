package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeanprocha/backend-engine-go/internal/history"
)

// saveSimulationRecordHandler persiste uma simulação já concluída no Postgres.
// POST /simulation-records
func (s *Server) saveSimulationRecordHandler(w http.ResponseWriter, r *http.Request) {
	var req SimulationRecordCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "payload inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "user_id obrigatório")
		return
	}

	in := history.SaveInput{
		UserID:          strings.TrimSpace(req.UserID),
		OrganizationID: req.OrganizationID,
		Year:            req.Year,
		CompanyContext:  req.CompanyContext,
		Simulation: history.SimulationSnapshot{
			Year: req.Simulation.Year,
			Current: history.TaxBreakdownSnapshot{
				GrossTax: req.Simulation.Current.GrossTax,
				Credits:  req.Simulation.Current.Credits,
				NetTax:   req.Simulation.Current.NetTax,
			},
			Projected: history.TaxBreakdownSnapshot{
				GrossTax: req.Simulation.Projected.GrossTax,
				Credits:  req.Simulation.Projected.Credits,
				NetTax:   req.Simulation.Projected.NetTax,
			},
			Delta:    req.Simulation.Delta,
			DeltaPct: req.Simulation.DeltaPct,
		},
	}

	for _, svc := range req.Services {
		in.Services = append(in.Services, history.ServiceLine{
			Description: svc.Description,
			Amount:      svc.Amount,
			ISSRate:     svc.ISSRate,
		})
	}
	for _, ex := range req.Expenses {
		in.Expenses = append(in.Expenses, history.ExpenseLine{
			Description: ex.Description,
			Amount:      ex.Amount,
			IsEligible:  ex.IsEligible,
		})
	}
	for _, c := range req.Classifications {
		in.Classifications = append(in.Classifications, history.ClassificationLine{
			Description:   c.Description,
			IsEligible:    c.IsEligible,
			Confidence:    c.Confidence,
			Justification: c.Justification,
			LegalBase:     c.LegalBase,
			RiskLevel:     c.RiskLevel,
			RegimeType:    c.RegimeType,
		})
	}

	id, err := s.history.Save(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro ao salvar histórico: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, SimulationRecordCreateResponse{ID: id.String()})
}

// listSimulationRecordsHandler lista simulações do usuário (header X-User-ID).
// GET /simulation-records?limit=20
func (s *Server) listSimulationRecordsHandler(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "header X-User-ID obrigatório")
		return
	}
	limit := 20
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}

	rows, err := s.history.ListByUser(r.Context(), userID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro ao listar: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, rows)
}

// getSimulationRecordHandler retorna uma simulação para reidratar o dashboard.
// GET /simulation-records/{id}
func (s *Server) getSimulationRecordHandler(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "header X-User-ID obrigatório")
		return
	}
	rawID := strings.TrimSpace(r.PathValue("id"))
	id, err := uuid.Parse(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}

	d, err := s.history.GetByID(r.Context(), userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro ao carregar: "+err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "simulação não encontrada")
		return
	}

	resp := SimulationRecordDetailResponse{
		ID:             d.ID.String(),
		CreatedAt:      d.CreatedAt.UTC().Format(time.RFC3339),
		Year:           d.Year,
		CompanyContext: d.CompanyContext,
		Simulation: SimulationResponse{
			Year: d.Simulation.Year,
			Current: TaxBreakdownResponse{
				GrossTax: d.Simulation.Current.GrossTax,
				Credits:  d.Simulation.Current.Credits,
				NetTax:   d.Simulation.Current.NetTax,
			},
			Projected: TaxBreakdownResponse{
				GrossTax: d.Simulation.Projected.GrossTax,
				Credits:  d.Simulation.Projected.Credits,
				NetTax:   d.Simulation.Projected.NetTax,
			},
			Delta:    d.Simulation.Delta,
			DeltaPct: d.Simulation.DeltaPct,
		},
	}

	for i, svc := range d.Services {
		resp.Services = append(resp.Services, FormServiceDTO{
			ID:          fmt.Sprintf("hist-svc-%d", i),
			Description: svc.Description,
			Amount:      svc.Amount,
			ISSRate:     svc.ISSRate,
		})
	}
	for i, ex := range d.Expenses {
		resp.Expenses = append(resp.Expenses, FormExpenseDTO{
			ID:          fmt.Sprintf("hist-exp-%d", i),
			Description: ex.Description,
			Amount:      ex.Amount,
		})
	}
	for _, cl := range d.Classifications {
		resp.Classifications = append(resp.Classifications, BatchClassificationItem{
			Description:   cl.Description,
			IsEligible:    cl.IsEligible,
			Confidence:    cl.Confidence,
			Justification: cl.Justification,
			LegalBase:     cl.LegalBase,
			RiskLevel:     cl.RiskLevel,
			RegimeType:    cl.RegimeType,
			Evidence:      []EvidenceArticleResponse{},
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
