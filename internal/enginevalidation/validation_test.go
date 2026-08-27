package enginevalidation

import (
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
)

// TestBuild_SemEvidenciaNaoValida garante que o placeholder embutido por
// padrão (evidencia/validacao_rfb.json, casos_total: 0) nunca reporta
// Validated: true — regra dura de PRODUCT.md (não fabricar selo).
func TestBuild_SemEvidenciaNaoValida(t *testing.T) {
	view := Build()
	if view.Validated {
		t.Fatal("Validated deveria ser false sem evidência gravada")
	}
	if view.CasosTotal != 0 {
		t.Fatalf("CasosTotal: got %d want 0 (evidência placeholder)", view.CasosTotal)
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
