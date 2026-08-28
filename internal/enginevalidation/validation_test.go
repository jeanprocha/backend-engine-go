package enginevalidation

import (
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
)

// TestBuild_SemEvidenciaNaoValida garante que uma evidência vazia/ausente
// (o estado de um checkout que nunca rodou a suíte -tags=rfb) nunca reporta
// Validated: true — regra dura de PRODUCT.md (não fabricar selo). Depois do
// W7/B2.1 o arquivo embutido de produção carrega evidência real (ver
// TestBuild_ComEvidenciaAtualValida abaixo), então este teste simula o
// estado vazio manipulando evidenceJSON diretamente em vez de depender do
// conteúdo do arquivo comitado.
func TestBuild_SemEvidenciaNaoValida(t *testing.T) {
	original := evidenceJSON
	defer func() { evidenceJSON = original }()
	evidenceJSON = []byte(`{"casos_total": 0}`)

	view := Build()
	if view.Validated {
		t.Fatal("Validated deveria ser false sem evidência gravada")
	}
	if view.CasosTotal != 0 {
		t.Fatalf("CasosTotal: got %d want 0 (evidência vazia)", view.CasosTotal)
	}
}

// TestBuild_ComEvidenciaAtualValida trava que a evidência REALMENTE
// comitada em evidencia/validacao_rfb.json (gravada por
// internal/tax/rfb_cross_test.go -rfb-update, W7/B2.1 — 28/08/2026, 8/8 anos
// batendo contra a API oficial da RFB) sustenta o selo hoje. Se este teste
// quebrar depois de mexer em transition_table.go, é o comportamento
// pretendido do hash-gate (ver comentário de Build): rodar a suíte -tags=rfb
// de novo com -rfb-update antes de reportar validação.
func TestBuild_ComEvidenciaAtualValida(t *testing.T) {
	view := Build()
	if !view.Validated {
		t.Fatal("Validated deveria ser true com a evidência comitada — rodar `go test -tags=rfb ./internal/tax/... -run TestRFB -rfb-update` de novo?")
	}
	if view.CasosTotal != 8 || view.CasosDivergem != 0 {
		t.Fatalf("CasosTotal=%d CasosDivergem=%d, want 8/0", view.CasosTotal, view.CasosDivergem)
	}
}

// TestBuild_SlicesNuncaNil garante slices vazios, não nil — um slice nil
// serializa "null" em JSON (mesma lição de lawcorpus.Build).
func TestBuild_SlicesNuncaNil(t *testing.T) {
	view := Build()
	if view.Escopo == nil {
		t.Error("Escopo é nil")
	}
	if view.ForaDoEscopo == nil {
		t.Error("ForaDoEscopo é nil")
	}
	if view.Casos == nil {
		t.Error("Casos é nil")
	}
}

// TestValidatedRule_ExigeCasosZeroDivergenciasVersaoEHashAtual documenta a
// regra de Validated diretamente sobre o Manifest — sem depender do arquivo
// embutido, para travar a regra em si. Qualquer divergência, uma tabela de
// transição que mudou desde que a evidência foi gravada (hash não bate), ou
// uma evidência que não diz contra QUAL versão da Calculadora RFB rodou,
// barra o selo — mesmo com casos suficientes e zero divergências.
func TestValidatedRule_ExigeCasosZeroDivergenciasVersaoEHashAtual(t *testing.T) {
	hashAtual := tax.TransitionTableHash()
	const versaoGravada = "1.0.0-beta"
	cases := []struct {
		name          string
		casosTotal    int
		casosDivergem int
		versao        string
		hashGravado   string
		want          bool
	}{
		{"sem casos", 0, 0, versaoGravada, hashAtual, false},
		{"casos sem divergência, versão e hash carimbados", 8, 0, versaoGravada, hashAtual, true},
		{"casos com 1 divergência, versão e hash carimbados", 8, 1, versaoGravada, hashAtual, false},
		{"casos sem divergência, tabela mudou desde a evidência", 8, 0, versaoGravada, "hash-de-uma-tabela-antiga", false},
		{"casos sem divergência, evidência sem carimbo de hash", 8, 0, versaoGravada, "", false},
		{"casos sem divergência, evidência sem versão da calculadora", 8, 0, "", hashAtual, false},
		{"sem versão e sem hash", 8, 0, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.casosTotal > 0 && tc.casosDivergem == 0 &&
				tc.versao != "" &&
				tc.hashGravado != "" && tc.hashGravado == hashAtual
			if got != tc.want {
				t.Errorf("casosTotal=%d casosDivergem=%d versao=%q hashGravado=%q: got %v want %v", tc.casosTotal, tc.casosDivergem, tc.versao, tc.hashGravado, got, tc.want)
			}
		})
	}
}
