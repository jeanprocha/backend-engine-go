package classifier

import (
	"context"
	"fmt"
	"strings"
)

const (
	strategyInsightMaxRunes = 250
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
	return string(r[:max])
}

// SimulationStrategyInsight gera texto curto de apoio à decisão (simulação educativa).
// Em falha de rede, resposta vazia ou erro da API, devolve StrategyInsightFallback e um erro
// para o handler registar em log.
func (s *Service) SimulationStrategyInsight(ctx context.Context, regime string, year int, current, projected TaxBreakdownSummary, delta, deltaPct, companyContext string) (string, error) {
	if s == nil || s.llm == nil {
		return StrategyInsightFallback, fmt.Errorf("strategy insight: serviço indisponível")
	}
	user := BuildStrategyUserMessage(regime, year, current, projected, delta, deltaPct, companyContext)
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
