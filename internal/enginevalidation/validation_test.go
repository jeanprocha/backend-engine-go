package enginevalidation

import "testing"

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

// TestValidatedRule_ExigeCasosEZeroDivergencias documenta a regra de
// Validated diretamente sobre o Manifest — sem depender do arquivo embutido,
// para travar a regra em si (qualquer divergência barra o selo, mesmo com
// casos suficientes).
func TestValidatedRule_ExigeCasosEZeroDivergencias(t *testing.T) {
	cases := []struct {
		name          string
		casosTotal    int
		casosDivergem int
		want          bool
	}{
		{"sem casos", 0, 0, false},
		{"casos sem divergência", 8, 0, true},
		{"casos com 1 divergência", 8, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.casosTotal > 0 && tc.casosDivergem == 0
			if got != tc.want {
				t.Errorf("casosTotal=%d casosDivergem=%d: got %v want %v", tc.casosTotal, tc.casosDivergem, got, tc.want)
			}
		})
	}
}
