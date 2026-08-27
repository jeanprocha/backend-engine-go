package http

import (
	"net/http"

	"github.com/jeanprocha/backend-engine-go/internal/enginevalidation"
)

// engineValidationHandler GET /engine/validation — pública (mesma postura de
// GET /law/corpus, só rate limiter). Reporta o que a suíte cruzada contra a
// Calculadora RFB REALMENTE mostrou (W7/B2.1-B2.3): o selo de validação no
// dossiê (frontend) nunca deve afirmar mais do que este endpoint sustenta
// (PRODUCT.md — "selos de validação... trabalho futuro não pode fabricar").
func (s *Server) engineValidationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
		return
	}

	view := enginevalidation.Build()

	cases := make([]EngineValidationCaseResponse, 0, len(view.Casos))
	for _, c := range view.Casos {
		cases = append(cases, EngineValidationCaseResponse{
			Year:       c.Year,
			CBSTribIA:  c.CBSTribIA,
			CBSRFB:     c.CBSRFB,
			IBSTribIA:  c.IBSTribIA,
			IBSRFB:     c.IBSRFB,
			Divergente: c.Divergente,
		})
	}

	var ref EngineValidationReferenceResponse
	if view.Validated {
		ref = EngineValidationReferenceResponse{
			Name:  "Calculadora de Tributos RFB/Serpro",
			URL:   view.CalculadoraURL,
			RunAt: view.ExecutadoEm,
		}
	}

	writeJSON(w, http.StatusOK, EngineValidationResponse{
		Validated:      view.Validated,
		Reference:      ref,
		Scope:          view.Escopo,
		OutOfScope:     view.ForaDoEscopo,
		Tolerance:      view.Tolerancia,
		Cases:          cases,
		CasesTotal:     view.CasosTotal,
		CasesDivergent: view.CasosDivergem,
	})
}
