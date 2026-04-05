package http

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

// lawArticleHandler GET /law/articles/{id}
// id é o article_id da linha em tax_law_chunks (ex.: evidência RAG), não só o título canónico.
func (s *Server) lawArticleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
		return
	}

	raw := strings.TrimSpace(r.PathValue("id"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "id do artigo é obrigatório")
		return
	}

	id, err := url.PathUnescape(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		writeError(w, http.StatusBadRequest, "id do artigo é obrigatório")
		return
	}

	art, err := s.store.GetFullArticleByChunkID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ingestion.ErrArticleNotFound) {
			writeError(w, http.StatusNotFound, "artigo não encontrado")
			return
		}
		slog.Error("law_article_fetch_failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "erro ao carregar artigo")
		return
	}

	writeJSON(w, http.StatusOK, LawArticleResponse{
		ID:      art.RequestedChunkID,
		Title:   art.Title,
		Content: art.Content,
		Source:  art.Source,
	})
}
