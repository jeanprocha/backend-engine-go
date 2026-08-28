package tax_test

import (
	"context"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
)

func canonicalTraceInput(companyRegime string) tax.SimulationInput {
	return tax.SimulationInput{
		Year:          2027,
		CompanyRegime: companyRegime,
		Services: []tax.Service{
			{ID: "svc-1", Description: "Consultoria", Amount: mustDecimal("10000.00"), ISSRate: mustDecimal("0.05"), RegimeType: "padrao"},
		},
		Expenses: []tax.Expense{
			{ID: "exp-1", Description: "Insumo elegível", Amount: mustDecimal("4000.00"), IsEligible: true, RegimeType: "padrao"},
			{ID: "exp-2", Description: "Despesa não elegível", Amount: mustDecimal("1000.00"), IsEligible: false, RegimeType: "padrao"},
		},
	}
}

// TestCalculate_TodosOsRamosProduzemTrace é o critério de pronto da PR 1
// ("trace presente em todos os ramos de Calculate") — table-driven sobre
// todo company_regime conhecido (company_regime.go:IsKnownCompanyRegime).
func TestCalculate_TodosOsRamosProduzemTrace(t *testing.T) {
	casos := []struct {
		companyRegime string
		wantRegime    string
	}{
		{"", tax.CompanyRegimeRegular},
		{tax.CompanyRegimeRegular, tax.CompanyRegimeRegular},
		{tax.CompanyRegimeMEI, tax.CompanyRegimeMEI},
		{tax.CompanyRegimeSimplesPuro, tax.CompanyRegimeSimplesPuro},
		{tax.CompanyRegimeSimplesHibrido, tax.CompanyRegimeSimplesHibrido},
		{tax.CompanyRegimeSectorDiferenciado60, tax.CompanyRegimeSectorDiferenciado60},
		{tax.CompanyRegimeAliquotaZero, tax.CompanyRegimeAliquotaZero},
		{tax.CompanyRegimeImobiliarioVenda, tax.CompanyRegimeImobiliarioVenda},
		{tax.CompanyRegimeImobiliarioAluguel, tax.CompanyRegimeImobiliarioAluguel},
		{tax.CompanyRegimeProfissionalLiberal, tax.CompanyRegimeProfissionalLiberal},
		{tax.CompanyRegimeExportadora, tax.CompanyRegimeExportadora},
		{tax.CompanyRegimeEntidadeImune, tax.CompanyRegimeEntidadeImune},
	}

	calc := tax.NewCalculator()
	for _, c := range casos {
		t.Run("regime="+c.companyRegime, func(t *testing.T) {
			result, err := calc.Calculate(context.Background(), canonicalTraceInput(c.companyRegime))
			if err != nil {
				t.Fatalf("Calculate: %v", err)
			}
			if result.Regime != c.wantRegime {
				t.Errorf("Regime: got %q, want %q", result.Regime, c.wantRegime)
			}
			if len(result.Current.Trace) == 0 {
				t.Error("Current.Trace vazio")
			}
			if len(result.Projected.Trace) == 0 {
				t.Error("Projected.Trace vazio")
			}
		})
	}
}

// TestCalculate_TraceReproduzOAgregado é o aceite literal do W2: o último
// passo de cada trace (o "Líquido...") tem que bater com NetTax — a prova de
// que o trace não é decoração, é a mesma conta.
func TestCalculate_TraceReproduzOAgregado(t *testing.T) {
	calc := tax.NewCalculator()
	result, err := calc.Calculate(context.Background(), canonicalTraceInput(tax.CompanyRegimeRegular))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	for nome, bd := range map[string]tax.TaxBreakdown{"Current": result.Current, "Projected": result.Projected} {
		if len(bd.Trace) == 0 {
			t.Fatalf("%s: trace vazio", nome)
		}
		last := bd.Trace[len(bd.Trace)-1]
		if !last.Output.Equal(bd.NetTax) {
			t.Errorf("%s: último passo do trace (%q) = %s, NetTax = %s — deveriam ser o mesmo número",
				nome, last.Label, last.Output, bd.NetTax)
		}
		if !last.Rounded {
			t.Errorf("%s: último passo do trace deveria estar marcado Rounded=true", nome)
		}
	}
}

// TestCalculate_TraceTemPassoDeCreditos garante que existe, em cada trace, um
// passo cujo Output bate com Credits — não só o líquido final.
func TestCalculate_TraceTemPassoDeCreditos(t *testing.T) {
	calc := tax.NewCalculator()
	result, err := calc.Calculate(context.Background(), canonicalTraceInput(tax.CompanyRegimeRegular))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	for nome, bd := range map[string]tax.TaxBreakdown{"Current": result.Current, "Projected": result.Projected} {
		found := false
		for _, step := range bd.Trace {
			if step.Item == "" && step.Output.Equal(bd.Credits) && step.Rounded {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: nenhum passo agregado do trace reproduz Credits (%s)", nome, bd.Credits)
		}
	}
}

// TestCalculate_TraceItemNomeiaAsLinhas garante que os passos por serviço e
// por despesa carregam o Item certo — é o que permite ao leitor achar, no
// trace, a linha específica que originou uma citação no dossiê.
func TestCalculate_TraceItemNomeiaAsLinhas(t *testing.T) {
	calc := tax.NewCalculator()
	result, err := calc.Calculate(context.Background(), canonicalTraceInput(tax.CompanyRegimeRegular))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	itemsOf := func(steps []tax.CalculationStep) map[string]bool {
		out := map[string]bool{}
		for _, s := range steps {
			if s.Item != "" {
				out[s.Item] = true
			}
		}
		return out
	}

	currentItems := itemsOf(result.Current.Trace)
	if !currentItems["Consultoria"] {
		t.Error("Current.Trace: nenhum passo com Item=\"Consultoria\" (o serviço)")
	}
	if !currentItems["Insumo elegível"] {
		t.Error("Current.Trace: nenhum passo com Item=\"Insumo elegível\" (a despesa elegível)")
	}
	if currentItems["Despesa não elegível"] {
		t.Error("Current.Trace: despesa NÃO elegível não deveria gerar passo de crédito")
	}

	projectedItems := itemsOf(result.Projected.Trace)
	if !projectedItems["Consultoria"] {
		t.Error("Projected.Trace: nenhum passo com Item=\"Consultoria\"")
	}
	if !projectedItems["Insumo elegível"] {
		t.Error("Projected.Trace: nenhum passo com Item=\"Insumo elegível\"")
	}
}

// TestCalculate_MEISemDecomposicao trava a decisão explícita de não fabricar
// componentes para MEI (ver comentário em calculator.go) — Components
// continua zero, mas o trace ainda existe (um passo, o DAS fixo).
func TestCalculate_MEISemDecomposicao(t *testing.T) {
	calc := tax.NewCalculator()
	result, err := calc.Calculate(context.Background(), tax.SimulationInput{
		Year:          2027,
		CompanyRegime: tax.CompanyRegimeMEI,
		Services:      []tax.Service{{ID: "svc-1", Description: "Serviço MEI", Amount: mustDecimal("1000.00"), ISSRate: mustDecimal("0.05")}},
	})
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if got := result.Current.Components.Sum(); !got.IsZero() {
		t.Errorf("Components deveria ser zero para MEI, veio %s", got)
	}
	if len(result.Current.Trace) != 1 {
		t.Errorf("esperava 1 passo no trace do MEI (DAS fixo), veio %d", len(result.Current.Trace))
	}
	if result.Regime != tax.CompanyRegimeMEI {
		t.Errorf("Regime: got %q, want %q", result.Regime, tax.CompanyRegimeMEI)
	}
}

// TestCalculate_ImobiliarioTraceMostraAAliquotaEfetiva garante que a
// derivação "padrão × multiplicador" (achado da auditoria de arredondamento
// em Round(6)) aparece como passo próprio, distinguindo venda (0,6) de
// aluguel (0,4).
func TestCalculate_ImobiliarioTraceMostraAAliquotaEfetiva(t *testing.T) {
	calc := tax.NewCalculator()
	for _, c := range []struct {
		regime string
		mult   string
	}{
		{tax.CompanyRegimeImobiliarioVenda, "0.6"},
		{tax.CompanyRegimeImobiliarioAluguel, "0.4"},
	} {
		t.Run(c.regime, func(t *testing.T) {
			input := canonicalTraceInput(c.regime)
			input.ImobiliarioRedutorAjusteBRL = mustDecimal("500.00")
			result, err := calc.Calculate(context.Background(), input)
			if err != nil {
				t.Fatalf("Calculate: %v", err)
			}
			found := false
			for _, step := range result.Projected.Trace {
				if step.Label == "Alíquota efetiva (imobiliário)" {
					found = true
					for _, in := range step.Inputs {
						if in.Name == "multiplicador_perfil" && in.Value.String() != c.mult {
							t.Errorf("multiplicador_perfil: got %s, want %s", in.Value.String(), c.mult)
						}
					}
				}
			}
			if !found {
				t.Error("passo \"Alíquota efetiva (imobiliário)\" ausente do trace")
			}
		})
	}
}
