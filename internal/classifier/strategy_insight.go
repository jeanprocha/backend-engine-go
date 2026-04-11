package classifier

import (
	"context"
	"fmt"
	"strings"
)

const (
	// Limite pós-LLM em runes: cartão «insight estratégico» no frontend mostra o texto integral (sem line-clamp).
	strategyInsightMaxRunes = 900
	// StrategyInsightFallback é retornado quando a LLM falha ou responde vazio.
	StrategyInsightFallback = "Analise créditos e fornecedores para otimizar o cenário projetado."
)

func truncateStrategyInsightRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	// Reserva 1 rune para reticências quando ainda assim exceder o teto (ex.: resposta anómala da API).
	return string(r[:max-1]) + "…"
}

// SimulationStrategyInsight gera parágrafo de apoio à decisão (simulação educativa), com teto em runes após a LLM.
// Em falha de rede, resposta vazia ou erro da API, devolve StrategyInsightFallback e um erro
// para o handler registar em log.
func (s *Service) SimulationStrategyInsight(ctx context.Context, regime string, year int, current, projected TaxBreakdownSummary, delta, deltaPct, companyContext, transitionFactorsJSON string) (string, error) {
	if s == nil || s.llm == nil {
		return StrategyInsightFallback, fmt.Errorf("strategy insight: serviço indisponível")
	}
	user := BuildStrategyUserMessage(regime, year, current, projected, delta, deltaPct, companyContext, transitionFactorsJSON)
	cr, err := s.llm.StrategyInsightChat(ctx, StrategySOP, user)
	if err != nil {
		return StrategyInsightFallback, fmt.Errorf("strategy insight: %w", err)
	}
	t := strings.TrimSpace(cr.Content)
	if t == "" {
		return StrategyInsightFallback, fmt.Errorf("strategy insight: resposta vazia")
	}
	return truncateStrategyInsightRunes(t, strategyInsightMaxRunes), nil
}
