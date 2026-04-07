package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jeanprocha/backend-engine-go/internal/classifier"
	"github.com/jeanprocha/backend-engine-go/internal/tax"
	"github.com/shopspring/decimal"
)

// STRATEGY_INSIGHT_ENABLED: quando "false" ou "0", POST /simulations não chama a LLM de insight
// (útil para stress test só do motor e baseline de latência). Qualquer outro valor ou vazio = ligado.
func strategyInsightEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("STRATEGY_INSIGHT_ENABLED")))
	return v != "false" && v != "0"
}

func creditLeakLLMEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("CREDIT_LEAK_LLM_ENABLED")))
	return v != "false" && v != "0"
}

// simulationHandler executa a simulação comparativa de carga tributária.
// POST /simulations
func (s *Server) simulationHandler(w http.ResponseWriter, r *http.Request) {
	var req SimulationRequest
	if !decodeJSONBody(w, r, &req) {
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

	redutor, err := resolveImobiliarioRedutor(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	baseInput := tax.SimulationInput{
		Year:                        req.Year,
		CompanyRegime:               req.CompanyRegime,
		CompanyContext:              req.CompanyContext,
		Services:                    services,
		Expenses:                    expenses,
		ImobiliarioRedutorAjusteBRL: redutor,
	}

	if !s.checkPipelineQuota(w, r) {
		return
	}

	start := time.Now()
	result, err := s.tax.Calculate(r.Context(), baseInput)
	if err != nil {
		writeInternalError(w, r, "simulation_calculate", err)
		return
	}

	series, err := tax.TransitionSeries(r.Context(), s.tax, baseInput)
	if err != nil {
		writeInternalError(w, r, "simulation_transition_series", err)
		return
	}

	out := toSimulationResponse(result)
	out.CompanyRegime = strings.TrimSpace(req.CompanyRegime)
	out.RevenueTotal = sumServiceRevenue(services).StringFixed(2)
	out.TransitionSeries = toTransitionSeriesPoints(series)

	llmOK := s.simulationLLMAllowed(r)
	if strategyInsightEnabled() && s.classifier != nil && !llmOK {
		slog.Info("simulation_llm_skipped",
			"reason", "no_authenticated_sub",
			"feature", "strategy_insight",
			"path", r.URL.Path,
		)
	}

	hasInsight := false
	if strategyInsightEnabled() && s.classifier != nil && llmOK {
		cur := classifier.TaxBreakdownSummary{
			GrossTax: out.Current.GrossTax,
			Credits:  out.Current.Credits,
			NetTax:   out.Current.NetTax,
		}
		proj := classifier.TaxBreakdownSummary{
			GrossTax: out.Projected.GrossTax,
			Credits:  out.Projected.Credits,
			NetTax:   out.Projected.NetTax,
		}
		insight, err := s.classifier.SimulationStrategyInsight(
			r.Context(),
			out.CompanyRegime,
			out.Year,
			cur,
			proj,
			out.Delta,
			out.DeltaPct,
			req.CompanyContext,
		)
		out.StrategyInsight = insight
		hasInsight = strings.TrimSpace(insight) != ""
		if err != nil {
			slog.Warn("strategy_insight_failed",
				"err", err.Error(),
				"year", req.Year,
				"company_regime", out.CompanyRegime,
			)
		}
	}

	if tax.CreditLeaksSupported(req.CompanyRegime) {
		leaks := tax.BuildCreditLeaks(req.Year, expenses)
		if len(leaks) > 0 {
			items := make([]classifier.CreditLeakEnrichmentItem, len(leaks))
			for i, L := range leaks {
				items[i] = classifier.CreditLeakEnrichmentItem{
					Description: L.Description,
					Value:       L.Value.StringFixed(2),
					LostCredit:  L.LostCredit.StringFixed(2),
					RegimeType:  L.RegimeType,
				}
			}
			final := items
			if creditLeakLLMEnabled() && s.classifier != nil && !llmOK {
				slog.Info("simulation_llm_skipped",
					"reason", "no_authenticated_sub",
					"feature", "credit_leak_enrich",
					"path", r.URL.Path,
				)
			}
			if creditLeakLLMEnabled() && s.classifier != nil && llmOK {
				enriched, err := s.classifier.EnrichCreditLeaks(r.Context(), req.CompanyRegime, req.CompanyContext, items)
				if err != nil {
					slog.Warn("credit_leak_enrich_failed", "err", err.Error(), "leak_count", len(items))
				} else {
					final = enriched
				}
			}
			out.CreditLeaks = make([]CreditLeakResponse, len(final))
			for i, f := range final {
				out.CreditLeaks[i] = CreditLeakResponse{
					Description: f.Description,
					Value:       f.Value,
					LostCredit:  f.LostCredit,
					Reason:      f.Reason,
					Fix:         f.Fix,
					RegimeType:  f.RegimeType,
				}
			}
		}
	}

	slog.Info("simulation_completed",
		"company_regime", strings.TrimSpace(req.CompanyRegime),
		"year", req.Year,
		"latency_ms", time.Since(start).Milliseconds(),
		"services", len(req.Services),
		"expenses", len(req.Expenses),
		"has_strategy_insight", hasInsight,
		"strategy_insight_len", len([]rune(strings.TrimSpace(out.StrategyInsight))),
	)

	s.recordSimulationPlg(r)
	writeJSON(w, http.StatusOK, out)
}

func resolveImobiliarioRedutor(req SimulationRequest) (decimal.Decimal, error) {
	if !tax.IsImobiliarioProfile(req.CompanyRegime) {
		return decimal.Zero, nil
	}
	s := strings.TrimSpace(req.ImobiliarioRedutorAjusteBRL)
	if s != "" {
		d, err := decimal.NewFromString(s)
		if err != nil {
			return decimal.Zero, fmt.Errorf("imobiliario_redutor_ajuste_brl inválido: %w", err)
		}
		if d.IsNegative() {
			return decimal.Zero, fmt.Errorf("imobiliario_redutor_ajuste_brl não pode ser negativo")
		}
		return d, nil
	}
	return tax.ImobiliarioRedutorDefaultBRL(req.CompanyRegime), nil
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
			RegimeType:  s.RegimeType,
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
			RegimeType:  e.RegimeType,
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

func sumServiceRevenue(services []tax.Service) decimal.Decimal {
	var sum decimal.Decimal
	for _, s := range services {
		sum = sum.Add(s.Amount)
	}
	return sum.Round(2)
}

func toTransitionSeriesPoints(results []tax.SimulationResult) []TransitionSeriesPoint {
	out := make([]TransitionSeriesPoint, 0, len(results))
	for _, r := range results {
		total := r.Current.NetTax.Add(r.Projected.NetTax).Round(2)
		out = append(out, TransitionSeriesPoint{
			Year:        r.Year,
			OldTaxNet:   r.Current.NetTax.StringFixed(2),
			NewTaxNet:   r.Projected.NetTax.StringFixed(2),
			TotalTaxNet: total.StringFixed(2),
		})
	}
	return out
}
