package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
	"github.com/shopspring/decimal"
)

// simulationHandler executa a simulação comparativa de carga tributária.
// POST /simulations
func (s *Server) simulationHandler(w http.ResponseWriter, r *http.Request) {
	var req SimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "payload inválido: "+err.Error())
		return
	}

	if req.Year < 2026 || req.Year > 2033 {
		writeError(w, http.StatusBadRequest, "year deve estar entre 2026 e 2033")
		return
	}
	if len(req.Services) == 0 {
		writeError(w, http.StatusBadRequest, "services não pode ser vazio")
		return
	}

	services, err := toTaxServices(req.Services)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	expenses, err := toTaxExpenses(req.Expenses)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.tax.Calculate(r.Context(), tax.SimulationInput{
		Year:     req.Year,
		Services: services,
		Expenses: expenses,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro no cálculo: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toSimulationResponse(result))
}

func toTaxServices(inputs []ServiceInput) ([]tax.Service, error) {
	out := make([]tax.Service, 0, len(inputs))
	for i, s := range inputs {
		amount, err := decimal.NewFromString(s.Amount)
		if err != nil {
			return nil, fmt.Errorf("services[%d].amount inválido: %w", i, err)
		}
		issRate, err := decimal.NewFromString(s.ISSRate)
		if err != nil {
			return nil, fmt.Errorf("services[%d].iss_rate inválido: %w", i, err)
		}
		out = append(out, tax.Service{
			ID:          fmt.Sprintf("svc-%d", i+1),
			Description: s.Description,
			Amount:      amount,
			ISSRate:     issRate,
		})
	}
	return out, nil
}

func toTaxExpenses(inputs []ExpenseInput) ([]tax.Expense, error) {
	out := make([]tax.Expense, 0, len(inputs))
	for i, e := range inputs {
		amount, err := decimal.NewFromString(e.Amount)
		if err != nil {
			return nil, fmt.Errorf("expenses[%d].amount inválido: %w", i, err)
		}
		out = append(out, tax.Expense{
			ID:          fmt.Sprintf("exp-%d", i+1),
			Description: e.Description,
			Amount:      amount,
			IsEligible:  e.IsEligible,
		})
	}
	return out, nil
}

func toSimulationResponse(r tax.SimulationResult) SimulationResponse {
	return SimulationResponse{
		Year:      r.Year,
		Current:   toBreakdownResponse(r.Current),
		Projected: toBreakdownResponse(r.Projected),
		Delta:     r.Delta.StringFixed(2),
		DeltaPct:  r.DeltaPct.StringFixed(2),
	}
}

func toBreakdownResponse(b tax.TaxBreakdown) TaxBreakdownResponse {
	return TaxBreakdownResponse{
		GrossTax: b.GrossTax.StringFixed(2),
		Credits:  b.Credits.StringFixed(2),
		NetTax:   b.NetTax.StringFixed(2),
	}
}
