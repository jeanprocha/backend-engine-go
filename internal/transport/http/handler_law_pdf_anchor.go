package http

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/auth"
	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
	"github.com/jeanprocha/backend-engine-go/internal/plg"
)

// lawPdfAnchorHandler GET /law/articles/{id}/pdf-anchor — Pro/Premium; requer autenticação.
func (s *Server) lawPdfAnchorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "nao autenticado")
		return
	}

	r, err := s.injectPlgTierIntoRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "token invalido")
		return
	}
	tier, _ := PlgTierFromContext(r.Context())
	if tier == plg.PlanFree {
		writeError(w, http.StatusForbidden, "ancoragem PDF disponivel nos planos Pro e Premium")
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

	page, coordY, convention, leiVer, err := s.store.GetPdfAnchorForChunk(r.Context(), id)
	if err != nil {
		if errors.Is(err, ingestion.ErrPdfAnchorNotFound) {
			writeError(w, http.StatusNotFound, "ancoragem PDF indisponível para este chunk")
			return
		}
		writeInternalError(w, r, "law_pdf_anchor", err)
		return
	}

	pdfURL := strings.TrimSpace(os.Getenv("LC68_OFFICIAL_PDF_URL"))
	if pdfURL == "" {
		writeError(w, http.StatusServiceUnavailable, "URL do PDF oficial não configurada (LC68_OFFICIAL_PDF_URL)")
		return
	}

	writeJSON(w, http.StatusOK, LawPdfAnchorResponse{
		PdfURL:     pdfURL,
		Page:       page,
		PdfCoordY:  coordY,
		Convention: convention,
		LeiVersion: leiVer,
		PrfFile:    "DOC-PLP-682024-20240722.pdf",
	})
}
