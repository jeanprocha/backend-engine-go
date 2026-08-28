package http

import (
	"encoding/json"
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
	factorsJSON := transitionFactorsJSONForYear(out.TransitionSeries, out.Year)

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
			factorsJSON,
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

// overlapModelDualComparative identifica o modo TribIA: duas simulações completas por ano (legado vs CBS/IBS).
const overlapModelDualComparative = "dual_comparative_v1"

func toSimulationResponse(r tax.SimulationResult) SimulationResponse {
	return SimulationResponse{
		Year:         r.Year,
		Regime:       r.Regime,
		Current:      toBreakdownResponse(r.Current),
		Projected:    toBreakdownResponse(r.Projected),
		Delta:        r.Delta.StringFixed(2),
		DeltaPct:     r.DeltaPct.StringFixed(2),
		OverlapModel: overlapModelDualComparative,
	}
}

func resolveOverlapModel(s string) string {
	s = strings.TrimSpace(s)
	if s != "" {
		return s
	}
	return overlapModelDualComparative
}

// transitionFactorsJSONForYear devolve JSON de TransitionYearFactors para o ano da simulação (prompt de insight).
func transitionFactorsJSONForYear(series []TransitionSeriesPoint, year int) string {
	for _, p := range series {
		if p.Year == year && p.Factors != nil {
			b, err := json.Marshal(p.Factors)
			if err != nil {
				return ""
			}
			return string(b)
		}
	}
	return ""
}

func toBreakdownResponse(b tax.TaxBreakdown) TaxBreakdownResponse {
	out := TaxBreakdownResponse{
		GrossTax: b.GrossTax.StringFixed(2),
		Credits:  b.Credits.StringFixed(2),
		NetTax:   b.NetTax.StringFixed(2),
		Components: TaxComponentsResponse{
			Pis:    b.Components.PIS.StringFixed(2),
			Cofins: b.Components.COFINS.StringFixed(2),
			Iss:    b.Components.ISS.StringFixed(2),
			Cbs:    b.Components.CBS.StringFixed(2),
			Ibs:    b.Components.IBS.StringFixed(2),
		},
	}
	for _, step := range b.Trace {
		out.Trace = append(out.Trace, toCalculationStepResponse(step))
	}
	return out
}

// toCalculationStepResponse converte tax.CalculationStep para o DTO — Output e
// os Inputs usam .String() (precisão total), nunca StringFixed(2): um passo
// intermediário propositalmente não-arredondado (Rounded=false) perderia a
// informação que existe para mostrar se fosse truncado aqui.
func toCalculationStepResponse(s tax.CalculationStep) CalculationStepResponse {
	out := CalculationStepResponse{
		Item:    s.Item,
		Label:   s.Label,
		Formula: s.Formula,
		Output:  s.Output.String(),
		Rounded: s.Rounded,
	}
	for _, in := range s.Inputs {
		out.Inputs = append(out.Inputs, CalculationStepInputResponse{Name: in.Name, Value: in.Value.String()})
	}
	return out
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
		rules := tax.RulesForYear(r.Year)
		f := transitionYearFactorsFromRules(rules)
		out = append(out, TransitionSeriesPoint{
			Year:        r.Year,
			OldTaxNet:   r.Current.NetTax.StringFixed(2),
			NewTaxNet:   r.Projected.NetTax.StringFixed(2),
			TotalTaxNet: total.StringFixed(2),
			Current:     toBreakdownResponse(r.Current),
			Projected:   toBreakdownResponse(r.Projected),
			Delta:       r.Delta.StringFixed(2),
			DeltaPct:    r.DeltaPct.StringFixed(2),
			Factors:     &f,
		})
	}
	return out
}

func transitionYearFactorsFromRules(rules tax.TaxRules) TransitionYearFactors {
	issF := rules.ISSMunicipalTransitionFactor()
	model := "municipal_transition_lc68"
	if issF.Equal(decimal.NewFromInt(1)) {
		model = "input_static"
	}
	// Basis: proveniência auditada na Onda 2/PR 7 (W1) — TransitionYearBasis
	// já existia desde o W7/B2.2 e nunca era chamada em código de produção
	// (W2/PR2, achado 3 da Etapa C). Único ponto de leitura: quem monta o
	// histórico legado (enrichTransitionSeriesLegacy) chama esta mesma
	// função, então um registro reconstituído na leitura também ganha Basis.
	basis := tax.TransitionYearBasis(rules.Year)
	return TransitionYearFactors{
		Year:                  rules.Year,
		PisCofinsFactor:       rules.PISCOFINSFactor.StringFixed(6),
		CbsRate:               rules.CBSRate.StringFixed(6),
		IbsRate:               rules.IBSRate.StringFixed(6),
		CombinedProjectedRate: rules.CombinedProjectedRate().StringFixed(6),
		IssMunicipalFactor:    issF.StringFixed(6),
		IssModel:              model,
		Basis:                 &RuleBasisResponse{Kind: basis.Kind, Note: basis.Note},
	}
}

// enrichTransitionSeriesLegacy preenche factors e breakdown mínimo quando o JSONB do histórico
// foi gravado antes destes campos — evita exigir nova simulação só para auditoria PRO.
// O segundo retorno indica se houve alteração (GET deve expor transition_series_enriched).
//
// Nota (W7/B2.2): factors vem de tax.RulesForYear(p.Year), ou seja, sempre a
// tabela ATUAL — nunca a que estava em vigor quando o registro foi salvo. Um
// registro sem factors persistido ganha, na leitura, os fatores de hoje ao
// lado de valores monetários calculados com a tabela de então. Isso já era
// verdade antes desta PR corrigir a tabela; não é regressão, mas o gap ficou
// mais visível. OldTaxNet/NewTaxNet/Current/Projected nunca são recalculados
// aqui — só Delta/DeltaPct quando ausentes, e a partir dos totais persistidos,
// nunca da tabela nova. Snapshots não são reescritos (mesma disciplina do W1).
func enrichTransitionSeriesLegacy(pts []TransitionSeriesPoint) ([]TransitionSeriesPoint, bool) {
	changed := false
	for i := range pts {
		p := &pts[i]
		if p.Factors == nil || strings.TrimSpace(p.Factors.PisCofinsFactor) == "" {
			rules := tax.RulesForYear(p.Year)
			f := transitionYearFactorsFromRules(rules)
			p.Factors = &f
			changed = true
		}
		if strings.TrimSpace(p.Current.NetTax) == "" && strings.TrimSpace(p.OldTaxNet) != "" {
			ot := strings.TrimSpace(p.OldTaxNet)
			p.Current = TaxBreakdownResponse{GrossTax: "0", Credits: "0", NetTax: ot}
			changed = true
		}
		if strings.TrimSpace(p.Projected.NetTax) == "" && strings.TrimSpace(p.NewTaxNet) != "" {
			nt := strings.TrimSpace(p.NewTaxNet)
			p.Projected = TaxBreakdownResponse{GrossTax: "0", Credits: "0", NetTax: nt}
			changed = true
		}
		if strings.TrimSpace(p.Delta) == "" && strings.TrimSpace(p.OldTaxNet) != "" && strings.TrimSpace(p.NewTaxNet) != "" {
			o, err1 := decimal.NewFromString(strings.TrimSpace(p.OldTaxNet))
			n, err2 := decimal.NewFromString(strings.TrimSpace(p.NewTaxNet))
			if err1 == nil && err2 == nil {
				d := n.Sub(o).Round(2)
				p.Delta = d.StringFixed(2)
				if o.IsPositive() {
					p.DeltaPct = d.Div(o).Mul(decimal.NewFromInt(100)).Round(2).StringFixed(2)
				}
				changed = true
			}
		}
	}
	return pts, changed
}
