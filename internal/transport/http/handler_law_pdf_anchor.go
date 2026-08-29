package http

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/auth"
	"github.com/jeanprocha/backend-engine-go/internal/config"
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

	s.writeLawPdfAnchor(w, r)
}

// publicLawPdfAnchorHandler GET /public/law-articles/{id}/pdf-anchor — sem
// autenticação e sem gate de plano, para o dossiê compartilhável.
//
// Por que existe: o dossiê público é lido por quem NÃO tem conta (o cliente do
// consultor), e "abrir a lei na página exata" é a prova que ele existe para
// entregar. Pela rota autenticada, esse leitor recebia 401 e o botão respondia
// "Inicie sessão para abrir o PDF oficial" — um beco sem saída no ponto mais
// importante da peça.
//
// Por que é seguro abrir: a resposta é a ancoragem artigo→página de um PDF
// OFICIAL e público (Diário/Planalto). Não há dado do usuário, da empresa nem
// da simulação — só a posição de um dispositivo num documento que qualquer
// pessoa baixa do governo. O gate de plano continua valendo na rota autenticada:
// ele protege a conveniência dentro da ferramenta, não o parecer já emitido.
func (s *Server) publicLawPdfAnchorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
		return
	}
	s.writeLawPdfAnchor(w, r)
}

// writeLawPdfAnchor resolve a ancoragem do chunk e escreve a resposta.
// Compartilhado pelas duas portas acima — a diferença entre elas é só quem
// pode entrar, nunca o que é resolvido.
func (s *Server) writeLawPdfAnchor(w http.ResponseWriter, r *http.Request) {
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

	// O PDF é resolvido a partir do lei_pdf_version DO CHUNK, não de uma
	// config global (Onda 2/PR 6): com LC 68 e LC 214 coexistindo no corpus, um
	// dossiê antigo citando lc68_ precisa abrir o PDF do PLP 68, não o da
	// LC 214 numa página que ali não significa nada.
	prfFile := ingestion.PDFFileForLeiVersion(leiVer)
	if prfFile == "" {
		// Versão desconhecida (chunk anterior ao mapa, ou mapa novo sem entrada
		// aqui): cai no documento configurado, que é o comportamento de sempre.
		prfFile = config.LawOfficialPDFFile()
	}
	pdfURL := config.LawOfficialPDFURLFor(prfFile)
	if pdfURL == "" {
		writeError(w, http.StatusServiceUnavailable, "URL do PDF oficial não configurada (LAW_OFFICIAL_PDF_URL ou LAW_OFFICIAL_PDF_BASE_URL)")
		return
	}

	writeJSON(w, http.StatusOK, LawPdfAnchorResponse{
		PdfURL:     pdfURL,
		Page:       page,
		PdfCoordY:  coordY,
		Convention: convention,
		LeiVersion: leiVer,
		PrfFile:    prfFile,
	})
}
