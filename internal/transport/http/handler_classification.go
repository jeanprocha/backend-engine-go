package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/classifier"
)

// classificationHandler processa POST /credit-classifications.
// Recebe a descrição de uma despesa, aciona o classificador RAG+LLM
// e devolve o veredicto de elegibilidade a crédito de IBS/CBS.
func (s *Server) classificationHandler(w http.ResponseWriter, r *http.Request) {
	var req ClassificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "payload inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		writeError(w, http.StatusBadRequest, "campo 'description' é obrigatório")
		return
	}

	result, err := s.classifier.ClassifyExpense(r.Context(), req.Description, req.Context, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro na classificação: "+err.Error())
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

	writeJSON(w, http.StatusOK, ClassificationResponse{
		IsEligible:    result.IsEligible,
		Confidence:    result.Confidence,
		Justification: result.Justification,
		LegalBase:     result.LegalBase,
		RiskLevel:     result.RiskLevel,
		RegimeType:    result.RegimeType,
		Evidence:      evidence,
	})
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "payload inválido: "+err.Error())
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
		if br.Err == "" {
			processed++
		}
		responseItems = append(responseItems, item)
	}

	writeJSON(w, http.StatusOK, BatchClassificationResponse{
		Total:     len(batchResults),
		Processed: processed,
		Results:   responseItems,
	})
}
