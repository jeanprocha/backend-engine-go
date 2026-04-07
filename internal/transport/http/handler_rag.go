package http

import (
	"net/http"
)

const (
	defaultThreshold = 0.33
	defaultLimit     = 5
)

// ragHandler responde perguntas sobre a legislação usando busca semântica (RAG).
// POST /ai/explanations
func (s *Server) ragHandler(w http.ResponseWriter, r *http.Request) {
	var req ExplanationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "campo 'question' é obrigatório")
		return
	}

	if req.Threshold <= 0 {
		req.Threshold = defaultThreshold
	}
	if req.Limit <= 0 {
		req.Limit = defaultLimit
	}

	results, err := s.rag.Query(r.Context(), req.Question, req.Threshold, req.Limit)
	if err != nil {
		writeInternalError(w, r, "rag_query", err)
		return
	}

	resp := ExplanationResponse{
		Question: req.Question,
		Results:  make([]ExplanationResult, 0, len(results)),
	}

	for _, r := range results {
		resp.Results = append(resp.Results, ExplanationResult{
			ArticleID:  r.ArticleID,
			Content:    r.Content,
			Similarity: r.Similarity,
			Type:       r.Metadata["type"],
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
