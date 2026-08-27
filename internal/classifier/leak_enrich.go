package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// EnrichCreditLeaks preenche reason e fix via LLM; preserva description, value, lost_credit e regime_type dos itens de entrada.
func (s *Service) EnrichCreditLeaks(ctx context.Context, companyRegime, companyContext string, items []CreditLeakEnrichmentItem) ([]CreditLeakEnrichmentItem, error) {
	if len(items) == 0 {
		return items, nil
	}
	if s == nil || s.llm == nil {
		return nil, fmt.Errorf("credit leak enrich: serviço indisponível")
	}

	payload, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("credit leak enrich: marshal input: %w", err)
	}
	user := BuildLeakageUserMessage(companyRegime, companyContext, string(payload))

	start := time.Now()
	cr, err := s.llm.LeakEnrichChat(ctx, buildLeakageSOP(defaultLawLabel()), user)
	latencyMS := time.Since(start).Milliseconds()
	if err != nil {
		slog.Warn("credit_leak_enrich_llm_failed",
			"latency_ms", latencyMS,
			"leak_count", len(items),
			"error", err.Error(),
		)
		return nil, err
	}
	slog.Info("credit_leak_enrich_completed",
		"latency_ms", latencyMS,
		"leak_count", len(items),
		"total_tokens", cr.Usage.TotalTokens,
	)

	raw := strings.TrimSpace(cr.Content)
	if raw == "" {
		return nil, fmt.Errorf("credit leak enrich: resposta vazia")
	}

	var wrapper struct {
		Leaks []CreditLeakEnrichmentItem `json:"leaks"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		slog.Warn("credit_leak_enrich_parse_failed", "error", err.Error(), "content_redacted", redactForLog(raw, 200))
		return nil, fmt.Errorf("credit leak enrich: parse: %w", err)
	}

	out := make([]CreditLeakEnrichmentItem, len(items))
	for i := range items {
		out[i] = items[i]
		if i >= len(wrapper.Leaks) {
			continue
		}
		r := wrapper.Leaks[i]
		out[i].Reason = strings.TrimSpace(r.Reason)
		out[i].Fix = strings.TrimSpace(r.Fix)
		// Garantir que números permanecem os do Go (não confiar na LLM).
		out[i].Description = items[i].Description
		out[i].Value = items[i].Value
		out[i].LostCredit = items[i].LostCredit
		out[i].RegimeType = items[i].RegimeType
	}

	if len(wrapper.Leaks) != len(items) {
		slog.Warn("credit_leak_enrich_length_mismatch",
			"expected", len(items),
			"got", len(wrapper.Leaks),
		)
	}

	return out, nil
}
