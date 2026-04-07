package http

import (
	"net/http"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/strategytags"
)

func strategyRowsToResponse(rows []strategytags.Row) StrategyTagsListResponse {
	out := make([]StrategyTagResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, StrategyTagResponse{
			Pattern:     row.Pattern,
			Label:       row.Label,
			Category:    strings.TrimSpace(row.Category),
			ColorScheme: row.ColorScheme,
		})
	}
	return StrategyTagsListResponse{Tags: out}
}

// strategyTagsHandler GET /strategy-tags — lista padrões para chips (cache em memória MVP).
func (s *Server) strategyTagsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
		return
	}
	if s.strategyTagsRepo == nil {
		writeJSON(w, http.StatusOK, StrategyTagsListResponse{Tags: []StrategyTagResponse{}})
		return
	}
	if s.strategyTagsCache != nil {
		if rows, ok := s.strategyTagsCache.Get(); ok {
			writeJSON(w, http.StatusOK, strategyRowsToResponse(rows))
			return
		}
	}
	rows, err := s.strategyTagsRepo.ListAll(r.Context())
	if err != nil {
		writeInternalError(w, r, "strategy_tags_list", err)
		return
	}
	if s.strategyTagsCache != nil {
		s.strategyTagsCache.Set(rows)
	}
	writeJSON(w, http.StatusOK, strategyRowsToResponse(rows))
}
