package http

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/history"
	"github.com/jeanprocha/backend-engine-go/internal/tax"
	"github.com/shopspring/decimal"
)

// Etapa C, W2/PR2 (docs/roadmap-execucao.md 4.1): trace, componentes, regime
// e proveniência passam a chegar à API. Estes testes cobrem os dois riscos
// reais do PR — (1) o motor precisa alimentar o DTO corretamente, (2) o DTO
// precisa sobreviver ao round-trip pelo terceiro espelho, history.SimulationSnapshot,
// que é uma cópia manual campo a campo e não pega divergência em tempo de
// compilação.

func TestToSimulationResponse_IncluiRegime(t *testing.T) {
	eng := tax.NewCalculator()
	result, err := eng.Calculate(context.Background(), tax.SimulationInput{
		Year:          2027,
		CompanyRegime: tax.CompanyRegimeExportadora,
		Services:      []tax.Service{{ID: "s1", Description: "Consultoria", Amount: mustDecimalHTTP("1000.00"), ISSRate: mustDecimalHTTP("0.05")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := toSimulationResponse(result)
	if out.Regime != tax.CompanyRegimeExportadora {
		t.Errorf("Regime: got %q, want %q", out.Regime, tax.CompanyRegimeExportadora)
	}
}

func TestToBreakdownResponse_IncluiComponentsETrace(t *testing.T) {
	eng := tax.NewCalculator()
	result, err := eng.Calculate(context.Background(), tax.SimulationInput{
		Year: 2027,
		Services: []tax.Service{
			{ID: "s1", Description: "Consultoria", Amount: mustDecimalHTTP("10000.00"), ISSRate: mustDecimalHTTP("0.05"), RegimeType: "padrao"},
		},
		Expenses: []tax.Expense{
			{ID: "e1", Description: "Insumo", Amount: mustDecimalHTTP("2000.00"), IsEligible: true, RegimeType: "padrao"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := toBreakdownResponse(result.Current)

	if out.Components.Pis == "" || out.Components.Cofins == "" || out.Components.Iss == "" {
		t.Errorf("Components vazio: %+v", out.Components)
	}
	if len(out.Trace) == 0 {
		t.Fatal("Trace vazio")
	}
	// Comparação NUMÉRICA, não de string: o trace usa .String() (precisão
	// total) e NetTax usa .StringFixed(2) (convenção monetária) — "500" e
	// "500.00" são o mesmo valor com formatações diferentes, de propósito
	// (ver o comentário de toCalculationStepResponse).
	last := out.Trace[len(out.Trace)-1]
	lastVal, err := decimal.NewFromString(last.Output)
	if err != nil {
		t.Fatalf("Output do último passo não é decimal válido: %q", last.Output)
	}
	netVal, err := decimal.NewFromString(out.NetTax)
	if err != nil {
		t.Fatalf("NetTax não é decimal válido: %q", out.NetTax)
	}
	if !lastVal.Equal(netVal) {
		t.Errorf("último passo do trace (%s) != NetTax (%s)", last.Output, out.NetTax)
	}
}

func TestTransitionYearFactorsFromRules_IncluiBasis(t *testing.T) {
	rules := tax.RulesForYear(2029)
	f := transitionYearFactorsFromRules(rules)
	if f.Basis == nil {
		t.Fatal("Basis é nil")
	}
	validKinds := map[string]bool{"lei_calendario": true, "estimativa_oficial": true, "premissa_tribia": true}
	if !validKinds[f.Basis.Kind] {
		t.Errorf("Basis.Kind inválido: %q", f.Basis.Kind)
	}
	if f.Basis.Note == "" {
		t.Error("Basis.Note vazio")
	}
	// 2029 é o primeiro ano com a delegação da alíquota de referência ao
	// Senado/TCU (Art. 349, auditado na Onda 2/PR 7) — a nota tem que citar o
	// artigo, não só declarar o Kind.
	if !strings.Contains(f.Basis.Note, "Art. 349") {
		t.Errorf("Basis.Note de 2029 não cita o Art. 349: %q", f.Basis.Note)
	}
}

// TestBreakdownSnapshot_RoundTrip é o teste que a duplicação manual entre DTO
// HTTP e history.SimulationSnapshot pede: ida (DTO→Snapshot→JSON) e volta
// (JSON→Snapshot→DTO) têm que reproduzir o valor original. Sem isso, os
// campos novos poderiam vazar silenciosamente no meio do caminho — o
// compilador não pega uma cópia de campo esquecida entre dois structs soltos.
func TestBreakdownSnapshot_RoundTrip(t *testing.T) {
	original := TaxBreakdownResponse{
		GrossTax: "1425.00",
		Credits:  "100.00",
		NetTax:   "1325.00",
		Components: TaxComponentsResponse{
			Pis: "165.00", Cofins: "760.00", Iss: "500.00", Cbs: "0", Ibs: "0",
		},
		Trace: []CalculationStepResponse{
			{
				Item: "Consultoria", Label: "ISS do serviço", Formula: "valor × alíquota",
				Inputs: []CalculationStepInputResponse{{Name: "valor_servico", Value: "10000"}, {Name: "aliquota_iss", Value: "0.05"}},
				Output: "500", Rounded: false,
			},
			{Label: "Líquido do regime atual", Formula: "bruto − créditos", Output: "1325.00", Rounded: true},
		},
	}

	snap := toBreakdownSnapshot(original)
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var reloaded history.TaxBreakdownSnapshot
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	back := fromBreakdownSnapshot(reloaded)

	if back.GrossTax != original.GrossTax || back.Credits != original.Credits || back.NetTax != original.NetTax {
		t.Errorf("valores agregados divergiram: got %+v, want %+v", back, original)
	}
	if back.Components != original.Components {
		t.Errorf("Components divergiu: got %+v, want %+v", back.Components, original.Components)
	}
	if len(back.Trace) != len(original.Trace) {
		t.Fatalf("Trace: got %d passos, want %d", len(back.Trace), len(original.Trace))
	}
	for i := range original.Trace {
		if back.Trace[i].Label != original.Trace[i].Label || back.Trace[i].Output != original.Trace[i].Output {
			t.Errorf("passo %d divergiu: got %+v, want %+v", i, back.Trace[i], original.Trace[i])
		}
		if len(back.Trace[i].Inputs) != len(original.Trace[i].Inputs) {
			t.Errorf("passo %d: Inputs divergiu em tamanho: got %d, want %d", i, len(back.Trace[i].Inputs), len(original.Trace[i].Inputs))
		}
	}
}

// TestFromBreakdownSnapshot_RegistroAntigoSemComponentsOuTrace é o achado 10
// do plano da Etapa C aplicado aos campos desta PR: um JSONB gravado ANTES
// desta PR não tem "components" nem "trace" — o unmarshal não pode falhar, e
// o resultado tem que ser Components zerado e Trace nil (a seção não
// renderiza, não quebra).
func TestFromBreakdownSnapshot_RegistroAntigoSemComponentsOuTrace(t *testing.T) {
	antigo := []byte(`{"gross_tax":"1425.00","credits":"0","net_tax":"1425.00"}`)
	var snap history.TaxBreakdownSnapshot
	if err := json.Unmarshal(antigo, &snap); err != nil {
		t.Fatalf("unmarshal de registro antigo falhou: %v", err)
	}
	out := fromBreakdownSnapshot(snap)
	if out.NetTax != "1425.00" {
		t.Errorf("NetTax: got %q", out.NetTax)
	}
	if out.Components != (TaxComponentsResponse{}) {
		t.Errorf("Components deveria ser zero-value, veio %+v", out.Components)
	}
	if out.Trace != nil {
		t.Errorf("Trace deveria ser nil, veio %+v", out.Trace)
	}
}

// TestFromRuleBasisSnapshot_NilParaRegistroAntigo: idem, para a proveniência.
func TestFromRuleBasisSnapshot_NilParaRegistroAntigo(t *testing.T) {
	antigo := []byte(`{"year":2026,"pis_cofins_factor":"1.000000","cbs_rate":"0.009000","ibs_rate":"0.001000"}`)
	var snap history.TransitionYearFactorsSnapshot
	if err := json.Unmarshal(antigo, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := fromRuleBasisSnapshot(snap.Basis); got != nil {
		t.Errorf("esperava nil para registro antigo sem basis, veio %+v", got)
	}
}

func mustDecimalHTTP(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}
