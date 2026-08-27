// Package enginevalidation reporta o que GET /engine/validation sustenta
// sobre a validação do motor de cálculo contra a Calculadora de Tributos
// oficial da RFB/Serpro (W7/B2.1-B2.3 — docs/roadmap-execucao.md).
//
// Regra dura, mesma de internal/lawcorpus: o que se reporta aqui é sempre um
// FATO de execução (quantos casos, quantos divergiram, contra qual versão),
// nunca uma afirmação inventada — PRODUCT.md lista "selos de validação (o
// selo RFB do W7 ainda não existe)" entre as ausências que trabalho futuro
// não pode fabricar. Sem evidência gravada, Validated é sempre false — não
// há forma estática honesta de um selo de validação (ao contrário do
// fallback cosmético de lawcorpus, que só espelha o rótulo/URL de sempre).
//
// A evidência é gravada por internal/tax/rfb_cross_test.go (atrás de build
// tag `rfb`, roda manualmente contra uma instância local da calculadora) e
// embutida aqui via go:embed — não em testdata/, que o `go build` normal do
// binário de produção (cmd/api) não inclui.
package enginevalidation

import (
	_ "embed"
	"encoding/json"
)

//go:embed evidencia/validacao_rfb.json
var evidenceJSON []byte

// EvidenceCase é o veredito de um ano — mesmo shape que
// internal/tax/rfb_cross_test.go grava.
type EvidenceCase struct {
	Year        int    `json:"year"`
	CBSTribIA   string `json:"cbs_tribia"`
	CBSRFB      string `json:"cbs_rfb"`
	IBSTribIA   string `json:"ibs_tribia"`
	IBSRFB      string `json:"ibs_rfb"`
	Divergente  bool   `json:"divergente"`
	Observacoes string `json:"observacoes,omitempty"`
}

// Manifest é o shape gravado por -rfb-update — ver
// internal/tax/rfb_cross_test.go:rfbEvidenceManifest.
type Manifest struct {
	ExecutadoEm    string         `json:"executado_em"`
	CalculadoraURL string         `json:"calculadora_url"`
	Escopo         []string       `json:"escopo"`
	ForaDoEscopo   []string       `json:"fora_do_escopo"`
	Tolerancia     string         `json:"tolerancia_brl"`
	Casos          []EvidenceCase `json:"casos"`
	CasosTotal     int            `json:"casos_total"`
	CasosDivergem  int            `json:"casos_divergentes"`
}

// View é o resultado completo — mapeia 1:1 para o payload de GET /engine/validation.
type View struct {
	Validated      bool
	ExecutadoEm    string
	CalculadoraURL string
	Escopo         []string
	ForaDoEscopo   []string
	Tolerancia     string
	Casos          []EvidenceCase
	CasosTotal     int
	CasosDivergem  int
}

// Build lê a evidência embutida e decide Validated. Validated exige: pelo
// menos 1 caso executado E zero divergências — qualquer divergência é motivo
// para NÃO afirmar validação, mesmo que parcial (o selo não tem meio-termo:
// ou o motor reproduz a calculadora oficial nos casos testados, ou não
// afirma nada).
func Build() View {
	var m Manifest
	// Evidência ausente/corrompida nunca derruba a rota — cai no zero-value
	// (Validated: false), a mesma postura defensiva de lawcorpus.Build.
	_ = json.Unmarshal(evidenceJSON, &m)

	view := View{
		ExecutadoEm:    m.ExecutadoEm,
		CalculadoraURL: m.CalculadoraURL,
		Escopo:         m.Escopo,
		ForaDoEscopo:   m.ForaDoEscopo,
		Tolerancia:     m.Tolerancia,
		Casos:          m.Casos,
		CasosTotal:     m.CasosTotal,
		CasosDivergem:  m.CasosDivergem,
	}
	if view.Escopo == nil {
		view.Escopo = []string{}
	}
	if view.ForaDoEscopo == nil {
		view.ForaDoEscopo = []string{}
	}
	if view.Casos == nil {
		view.Casos = []EvidenceCase{}
	}
	view.Validated = view.CasosTotal > 0 && view.CasosDivergem == 0
	return view
}
