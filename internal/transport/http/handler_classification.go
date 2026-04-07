package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/classifier"
	"github.com/jeanprocha/backend-engine-go/internal/strategytags"
)

// classificationHandler processa POST /credit-classifications.
// Recebe a descrição de uma despesa, aciona o classificador RAG+LLM
// e devolve o veredicto de elegibilidade a crédito de IBS/CBS.
func (s *Server) classificationHandler(w http.ResponseWriter, r *http.Request) {
	var req ClassificationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		writeError(w, http.StatusBadRequest, "campo 'description' é obrigatório")
		return
	}

	if !s.checkPipelineQuota(w, r) {
		return
	}

	result, err := s.classifier.ClassifyExpense(r.Context(), req.Description, req.Context, "")
	if err != nil {
		writeInternalError(w, r, "classification_single", err)
		return
	}

	evidence := make([]EvidenceArticleResponse, 0, len(result.Evidence))
	for _, e := range result.Evidence {
		evidence = append(evidence, EvidenceArticleResponse{
			ArticleID:  e.ArticleID,
			Content:    e.Content,
			Similarity: e.Similarity,
		})
	}

	resp := ClassificationResponse{
		IsEligible:    result.IsEligible,
		Confidence:    result.Confidence,
		Justification: result.Justification,
		LegalBase:     result.LegalBase,
		RiskLevel:     result.RiskLevel,
		RegimeType:    result.RegimeType,
		Evidence:      evidence,
	}
	if result.MatchedSpan != nil {
		resp.MatchedSpan = &MatchedSpanResponse{
			Start: result.MatchedSpan.Start,
			End:   result.MatchedSpan.End,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

const (
	defaultBatchConcurrency = 5
	maxBatchConcurrency     = 10
)

// classificationBatchHandler processa POST /credit-classifications/batch.
// Executa as classificações em paralelo com controle de concorrência via semáforo,
// garantindo que erros individuais não abortem o lote inteiro.
func (s *Server) classificationBatchHandler(w http.ResponseWriter, r *http.Request) {
	var req BatchClassificationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.Expenses) == 0 {
		writeError(w, http.StatusBadRequest, "campo 'expenses' não pode ser vazio")
		return
	}

	concurrency := req.MaxConcurrency
	if concurrency <= 0 {
		concurrency = defaultBatchConcurrency
	}
	if concurrency > maxBatchConcurrency {
		concurrency = maxBatchConcurrency
	}

	items := make([]classifier.BatchItem, 0, len(req.Expenses))
	for _, e := range req.Expenses {
		if strings.TrimSpace(e.Description) == "" {
			writeError(w, http.StatusBadRequest, "todas as despesas precisam de 'description' não vazia")
			return
		}
		items = append(items, classifier.BatchItem{
			ClientID:       strings.TrimSpace(e.ClientID),
			Description:    e.Description,
			CompanyContext: e.Context,
		})
	}

	if !s.checkPipelineQuota(w, r) {
		return
	}

	batchResults := s.classifier.ClassifyBatch(r.Context(), items, concurrency)

	processed := 0
	responseItems := make([]BatchClassificationItem, 0, len(batchResults))
	for _, br := range batchResults {
		item := BatchClassificationItem{
			ClientID:      br.ClientID,
			Description:   br.Description,
			IsEligible:    br.IsEligible,
			Confidence:    br.Confidence,
			Justification: br.Justification,
			LegalBase:     br.LegalBase,
			RiskLevel:     br.RiskLevel,
			RegimeType:    br.RegimeType,
			Error:         br.Err,
		}
		if len(br.Evidence) > 0 {
			ev := make([]EvidenceArticleResponse, 0, len(br.Evidence))
			for _, e := range br.Evidence {
				ev = append(ev, EvidenceArticleResponse{
					ArticleID:  e.ArticleID,
					Content:    e.Content,
					Similarity: e.Similarity,
				})
			}
			item.Evidence = ev
		}
		if br.MatchedSpan != nil {
			item.MatchedSpan = &MatchedSpanResponse{
				Start: br.MatchedSpan.Start,
				End:   br.MatchedSpan.End,
			}
		}
		if br.Err == "" {
			processed++
		}
		responseItems = append(responseItems, item)
	}

	discovered := discoverStrategyTagsFromBatch(r.Context(), s.strategyTagsRepo, s.strategyTagsCache, batchResults)

	writeJSON(w, http.StatusOK, BatchClassificationResponse{
		Total:          len(batchResults),
		Processed:      processed,
		Results:        responseItems,
		DiscoveredTags: discovered,
	})
}

// discoverStrategyTagsFromBatch deduplica sugestões da LLM, persiste com ON CONFLICT DO NOTHING
// e devolve apenas as linhas realmente inseridas.
func discoverStrategyTagsFromBatch(ctx context.Context, repo *strategytags.Repo, cache *strategytags.ListCache, results []classifier.BatchResult) []StrategyTagResponse {
	if repo == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var ordered []classifier.SuggestedTag
	for _, br := range results {
		if br.Err != "" {
			continue
		}
		for _, t := range br.SuggestedTags {
			p := strategytags.NormalizePattern(t.Pattern)
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			ordered = append(ordered, t)
		}
	}
	var discovered []StrategyTagResponse
	for _, t := range ordered {
		inserted, err := repo.InsertIgnore(ctx, t.Pattern, t.Label, t.Category, t.ColorScheme)
		if err != nil {
			slog.Warn("strategy_tag_insert_skipped", "err", err.Error(), "pattern", strategytags.NormalizePattern(t.Pattern))
			continue
		}
		if !inserted {
			continue
		}
		p := strategytags.NormalizePattern(t.Pattern)
		// Telemetria servidor: só pattern + label taxonómicos (sem contexto do cliente).
		slog.Info("strategy_tag_confirmed", "pattern", p, "label", strings.TrimSpace(t.Label))
		discovered = append(discovered, StrategyTagResponse{
			Pattern:     p,
			Label:       strings.TrimSpace(t.Label),
			Category:    strings.TrimSpace(t.Category),
			ColorScheme: strategytags.SanitizeColorScheme(t.ColorScheme),
		})
	}
	if len(discovered) > 0 && cache != nil {
		cache.Invalidate()
	}
	return discovered
}
