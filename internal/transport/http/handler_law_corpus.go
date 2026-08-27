package http

import (
	"net/http"
	"os"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/lawcorpus"
)

// lawCorpusHandler GET /law/corpus — pública (mesma postura de GET
// /law/articles/{id}, só rate limiter). Reporta o corpus REALMENTE
// ingerido: o selo de data-base no dossiê (features/legal-corpus no
// frontend) nunca deve afirmar mais do que este endpoint sustenta
// (PRODUCT.md).
func (s *Server) lawCorpusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
		return
	}

	rows, err := s.store.ListCorpusDocuments(r.Context())
	if err != nil {
		writeInternalError(w, r, "law_corpus_list", err)
		return
	}

	override := strings.TrimSpace(os.Getenv("LAW_CORPUS_CURRENT_SOURCE"))
	view := lawcorpus.Build(rows, override)

	docs := make([]LawCorpusDocumentResponse, 0, len(view.Documents))
	for _, d := range view.Documents {
		docs = append(docs, LawCorpusDocumentResponse{
			ID:          d.ID,
			Label:       d.Label,
			Version:     d.Version,
			PublishedAt: d.PublishedAt,
			SourceURL:   d.SourceURL,
			ChunkPrefix: d.ChunkPrefix,
		})
	}

	changelog := make([]LawCorpusChangelogEntryResponse, 0, len(view.Changelog))
	for _, c := range view.Changelog {
		changelog = append(changelog, LawCorpusChangelogEntryResponse{
			Type:  c.Type,
			Label: c.Label,
			Desc:  c.Desc,
		})
	}

	writeJSON(w, http.StatusOK, LawCorpusResponse{
		Documents:         docs,
		CurrentDocumentID: view.CurrentDocumentID,
		Changelog:         changelog,
	})
}
