package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeanprocha/backend-engine-go/internal/auth"
	"github.com/jeanprocha/backend-engine-go/internal/history"
)

// saveSimulationRecordHandler persiste uma simulação já concluída no Postgres.
// POST /simulation-records
func (s *Server) saveSimulationRecordHandler(w http.ResponseWriter, r *http.Request) {
	var req SimulationRecordCreateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "nao autenticado")
		return
	}

	regime := strings.TrimSpace(req.CompanyRegime)
	if regime == "" {
		regime = strings.TrimSpace(req.Simulation.CompanyRegime)
	}

	companyID := req.CompanyID
	if companyID == nil {
		companyID = req.OrganizationID
	}

	in := history.SaveInput{
		UserID:         strings.TrimSpace(userID),
		CompanyID:      companyID,
		Year:           req.Year,
		CompanyContext: req.CompanyContext,
		Simulation: history.SimulationSnapshot{
			Year:             req.Simulation.Year,
			CompanyRegime:    regime,
			StrategyInsight:  strings.TrimSpace(req.Simulation.StrategyInsight),
			RevenueTotal:     strings.TrimSpace(req.Simulation.RevenueTotal),
			OverlapModel:     resolveOverlapModel(req.Simulation.OverlapModel),
			TransitionSeries: snapshotTransitionSeriesFromDTO(req.Simulation.TransitionSeries),
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
			Delta:       req.Simulation.Delta,
			DeltaPct:    req.Simulation.DeltaPct,
			CreditLeaks: snapshotCreditLeaksFromDTO(req.Simulation.CreditLeaks),
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
	classItems := req.Classifications
	if req.ClassificationsSnapshot != nil && len(req.ClassificationsSnapshot.ExpenseClassifications) > 0 {
		classItems = req.ClassificationsSnapshot.ExpenseClassifications
	}
	for _, c := range classItems {
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
	if req.ClassificationsSnapshot != nil {
		raw, err := json.Marshal(req.ClassificationsSnapshot)
		if err != nil {
			writeError(w, http.StatusBadRequest, "classifications_snapshot inválido")
			return
		}
		in.ClassificationsSnapshot = raw
	}

	id, err := s.history.Save(r.Context(), in)
	if err != nil {
		writeInternalError(w, r, "history_save", err)
		return
	}

	writeJSON(w, http.StatusCreated, SimulationRecordCreateResponse{ID: id.String()})
}

// listSimulationRecordsHandler lista simulações do usuário (JWT ou X-User-ID em AUTH_SKIP).
// GET /simulation-records?limit=20
func (s *Server) listSimulationRecordsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "nao autenticado")
		return
	}
	userID = strings.TrimSpace(userID)
	limit := 20
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}

	rows, err := s.history.ListByUser(r.Context(), userID, limit)
	if err != nil {
		writeInternalError(w, r, "history_list", err)
		return
	}

	writeJSON(w, http.StatusOK, rows)
}

// getSimulationRecordHandler retorna uma simulação para reidratar o dashboard.
// GET /simulation-records/{id}
func (s *Server) getSimulationRecordHandler(w http.ResponseWriter, r *http.Request) {
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

	d, err := s.history.GetByID(r.Context(), userID, id)
	if err != nil {
		writeInternalError(w, r, "history_get", err)
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "simulação não encontrada")
		return
	}

	writeJSON(w, http.StatusOK, simulationRecordDetailFromHistory(d))
}

// getPublicSimulationRecordHandler — GET /public/simulation-records/{id} (sem autenticação; o UUID é o segredo de partilha).
func (s *Server) getPublicSimulationRecordHandler(w http.ResponseWriter, r *http.Request) {
	rawID := strings.TrimSpace(r.PathValue("id"))
	id, err := uuid.Parse(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	d, err := s.history.GetByIDPublic(r.Context(), id)
	if err != nil {
		writeInternalError(w, r, "history_get_public", err)
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "simulação não encontrada")
		return
	}
	writeJSON(w, http.StatusOK, simulationRecordDetailFromHistory(d))
}

func simulationRecordDetailFromHistory(d *history.Detail) SimulationRecordDetailResponse {
	ts, transitionEnriched := enrichTransitionSeriesLegacy(
		transitionSeriesDTOFromSnapshot(d.Simulation.TransitionSeries),
	)
	resp := SimulationRecordDetailResponse{
		ID:             d.ID.String(),
		CreatedAt:      d.CreatedAt.UTC().Format(time.RFC3339),
		Year:           d.Year,
		CompanyContext: d.CompanyContext,
		CompanyRegime:  strings.TrimSpace(d.Simulation.CompanyRegime),
		Simulation: SimulationResponse{
			Year:                     d.Simulation.Year,
			CompanyRegime:            strings.TrimSpace(d.Simulation.CompanyRegime),
			StrategyInsight:          strings.TrimSpace(d.Simulation.StrategyInsight),
			RevenueTotal:             strings.TrimSpace(d.Simulation.RevenueTotal),
			OverlapModel:             resolveOverlapModel(strings.TrimSpace(d.Simulation.OverlapModel)),
			TransitionSeries:         ts,
			TransitionSeriesEnriched: transitionEnriched,
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
			Delta:       d.Simulation.Delta,
			DeltaPct:    d.Simulation.DeltaPct,
			CreditLeaks: creditLeaksDTOFromSnapshot(d.Simulation.CreditLeaks),
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
	if len(d.ClassificationsSnapshot) > 0 {
		resp.ClassificationsSnapshot = json.RawMessage(d.ClassificationsSnapshot)
		var snap ClassificationHistorySnapshot
		if err := json.Unmarshal(d.ClassificationsSnapshot, &snap); err == nil && len(snap.ExpenseClassifications) > 0 {
			resp.Classifications = append(resp.Classifications, snap.ExpenseClassifications...)
		}
	}
	if len(resp.Classifications) == 0 {
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
	}
	return resp
}

func snapshotTransitionSeriesFromDTO(pts []TransitionSeriesPoint) []history.TransitionSeriesSnapshot {
	if len(pts) == 0 {
		return nil
	}
	out := make([]history.TransitionSeriesSnapshot, 0, len(pts))
	for _, p := range pts {
		s := history.TransitionSeriesSnapshot{
			Year:        p.Year,
			OldTaxNet:   p.OldTaxNet,
			NewTaxNet:   p.NewTaxNet,
			TotalTaxNet: p.TotalTaxNet,
			Delta:       p.Delta,
			DeltaPct:    p.DeltaPct,
		}
		if p.Current.GrossTax != "" || p.Current.Credits != "" || p.Current.NetTax != "" {
			s.Current = &history.TaxBreakdownSnapshot{
				GrossTax: p.Current.GrossTax,
				Credits:  p.Current.Credits,
				NetTax:   p.Current.NetTax,
			}
		}
		if p.Projected.GrossTax != "" || p.Projected.Credits != "" || p.Projected.NetTax != "" {
			s.Projected = &history.TaxBreakdownSnapshot{
				GrossTax: p.Projected.GrossTax,
				Credits:  p.Projected.Credits,
				NetTax:   p.Projected.NetTax,
			}
		}
		if p.Factors != nil {
			s.Factors = &history.TransitionYearFactorsSnapshot{
				Year:                  p.Factors.Year,
				PisCofinsFactor:       p.Factors.PisCofinsFactor,
				CbsRate:               p.Factors.CbsRate,
				IbsRate:               p.Factors.IbsRate,
				CombinedProjectedRate: p.Factors.CombinedProjectedRate,
				IssMunicipalFactor:    p.Factors.IssMunicipalFactor,
				IssModel:              p.Factors.IssModel,
			}
		}
		out = append(out, s)
	}
	return out
}

func transitionSeriesDTOFromSnapshot(pts []history.TransitionSeriesSnapshot) []TransitionSeriesPoint {
	if len(pts) == 0 {
		return nil
	}
	out := make([]TransitionSeriesPoint, 0, len(pts))
	for _, p := range pts {
		pt := TransitionSeriesPoint{
			Year:        p.Year,
			OldTaxNet:   p.OldTaxNet,
			NewTaxNet:   p.NewTaxNet,
			TotalTaxNet: p.TotalTaxNet,
			Delta:       p.Delta,
			DeltaPct:    p.DeltaPct,
		}
		if p.Current != nil {
			pt.Current = TaxBreakdownResponse{
				GrossTax: p.Current.GrossTax,
				Credits:  p.Current.Credits,
				NetTax:   p.Current.NetTax,
			}
		}
		if p.Projected != nil {
			pt.Projected = TaxBreakdownResponse{
				GrossTax: p.Projected.GrossTax,
				Credits:  p.Projected.Credits,
				NetTax:   p.Projected.NetTax,
			}
		}
		if p.Factors != nil {
			pt.Factors = &TransitionYearFactors{
				Year:                  p.Factors.Year,
				PisCofinsFactor:       p.Factors.PisCofinsFactor,
				CbsRate:               p.Factors.CbsRate,
				IbsRate:               p.Factors.IbsRate,
				CombinedProjectedRate: p.Factors.CombinedProjectedRate,
				IssMunicipalFactor:    p.Factors.IssMunicipalFactor,
				IssModel:              p.Factors.IssModel,
			}
		}
		out = append(out, pt)
	}
	return out
}

func snapshotCreditLeaksFromDTO(leaks []CreditLeakResponse) []history.CreditLeakSnapshot {
	if len(leaks) == 0 {
		return nil
	}
	out := make([]history.CreditLeakSnapshot, 0, len(leaks))
	for _, L := range leaks {
		out = append(out, history.CreditLeakSnapshot{
			Description: L.Description,
			Value:       L.Value,
			LostCredit:  L.LostCredit,
			Reason:      L.Reason,
			Fix:         L.Fix,
			RegimeType:  L.RegimeType,
		})
	}
	return out
}

func creditLeaksDTOFromSnapshot(leaks []history.CreditLeakSnapshot) []CreditLeakResponse {
	if len(leaks) == 0 {
		return nil
	}
	out := make([]CreditLeakResponse, 0, len(leaks))
	for _, L := range leaks {
		out = append(out, CreditLeakResponse{
			Description: L.Description,
			Value:       L.Value,
			LostCredit:  L.LostCredit,
			Reason:      L.Reason,
			Fix:         L.Fix,
			RegimeType:  L.RegimeType,
		})
	}
	return out
}
