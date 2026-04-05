package tax_test

import (
	"context"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
	"github.com/shopspring/decimal"
)

func mustDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic("mustDecimal: " + s + ": " + err.Error())
	}
	return d
}

func newCalc() tax.Engine {
	return tax.NewCalculator()
}

// TestCalculate_ZeroExpenses valida o caso base: empresa com receita mas sem despesas.
// Ano 2026: CBS = 0,9%, IBS = 0,1%, PIS = 1,65%, COFINS = 7,6%.
// Receita: R$ 10.000,00 | ISS: 5%.
//
// Esperado regime atual:
//   PIS+COFINS = 10000 * (0.0165 + 0.076) = 10000 * 0.0925 = 925.00
//   ISS        = 10000 * 0.05 = 500.00
//   Bruto atual = 1425.00 | Créditos = 0 | Líquido atual = 1425.00
//
// Esperado regime projetado:
//   CBS+IBS = 10000 * (0.009 + 0.001) = 10000 * 0.010 = 100.00
//   Créditos = 0 | Líquido projetado = 100.00
//
// Delta = 1425.00 - 100.00 = 1325.00 (economia)
func TestCalculate_ZeroExpenses(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{
				ID:          "svc-1",
				Description: "Serviço de TI",
				Amount:      mustDecimal("10000.00"),
				ISSRate:     mustDecimal("0.05"),
			},
		},
		Expenses: nil,
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate retornou erro inesperado: %v", err)
	}

	assertEqual(t, "Current.GrossTax", "1425.00", result.Current.GrossTax)
	assertEqual(t, "Current.Credits", "0", result.Current.Credits)
	assertEqual(t, "Current.NetTax", "1425.00", result.Current.NetTax)

	assertEqual(t, "Projected.GrossTax", "100.00", result.Projected.GrossTax)
	assertEqual(t, "Projected.Credits", "0", result.Projected.Credits)
	assertEqual(t, "Projected.NetTax", "100.00", result.Projected.NetTax)

	assertEqual(t, "Delta", "1325.00", result.Delta)
}

// TestCalculate_AllEligibleExpenses valida créditos integrais sobre despesas elegíveis.
// Receita: R$ 10.000,00 | Despesas elegíveis: R$ 5.000,00.
//
// Regime projetado (2026):
//   Bruto = 10000 * 0.010 = 100.00
//   Créditos = 5000 * 0.010 = 50.00
//   Líquido projetado = 50.00
func TestCalculate_AllEligibleExpenses(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{
				ID:      "svc-1",
				Amount:  mustDecimal("10000.00"),
				ISSRate: mustDecimal("0.05"),
			},
		},
		Expenses: []tax.Expense{
			{ID: "exp-1", Amount: mustDecimal("5000.00"), IsEligible: true},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate retornou erro inesperado: %v", err)
	}

	assertEqual(t, "Projected.GrossTax", "100.00", result.Projected.GrossTax)
	assertEqual(t, "Projected.Credits", "50.00", result.Projected.Credits)
	assertEqual(t, "Projected.NetTax", "50.00", result.Projected.NetTax)

	// Créditos no regime atual (PIS+COFINS sobre despesas elegíveis)
	// 5000 * 0.0925 = 462.50
	assertEqual(t, "Current.Credits", "462.50", result.Current.Credits)
}

// TestCalculate_MixedExpenses valida que apenas despesas marcadas como elegíveis
// geram crédito. Despesas não elegíveis devem ser ignoradas no cálculo.
//
// Despesas: R$ 3.000 elegível + R$ 2.000 não elegível.
// Esperado crédito projetado = 3000 * 0.010 = 30.00 (apenas a elegível).
func TestCalculate_MixedExpenses(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "svc-1", Amount: mustDecimal("10000.00"), ISSRate: mustDecimal("0.05")},
		},
		Expenses: []tax.Expense{
			{ID: "exp-1", Amount: mustDecimal("3000.00"), IsEligible: true},
			{ID: "exp-2", Amount: mustDecimal("2000.00"), IsEligible: false},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate retornou erro inesperado: %v", err)
	}

	// Crédito projetado apenas sobre exp-1
	assertEqual(t, "Projected.Credits", "30.00", result.Projected.Credits)
	// Crédito atual apenas sobre exp-1 (PIS+COFINS: 3000 * 0.0925 = 277.50)
	assertEqual(t, "Current.Credits", "277.50", result.Current.Credits)
}

// TestCalculate_NoServices valida que o motor rejeita input sem serviços.
func TestCalculate_NoServices(t *testing.T) {
	calc := newCalc()
	_, err := calc.Calculate(context.Background(), tax.SimulationInput{Year: 2026})
	if err == nil {
		t.Fatal("esperado erro para input sem serviços, mas não houve erro")
	}
}

// TestCalculate_RegimeDiferenciado60 valida a redução de 60% para saúde/educação.
//
// Serviço de saúde — diferenciado_60 — paga 40% da alíquota CBS+IBS padrão.
// Ano 2026: CBS+IBS padrão = 0.009 + 0.001 = 0.010 → efetiva = 0.010 * 0.4 = 0.004
// Receita: R$ 10.000,00
//
// Projetado bruto = 10000 * 0.004 = 40.00
// Sem créditos → líquido projetado = 40.00
// Padrão seria 100.00 → economia de 60%.
func TestCalculate_RegimeDiferenciado60(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{
				ID:          "svc-saude",
				Description: "Consulta médica",
				Amount:      mustDecimal("10000.00"),
				ISSRate:     mustDecimal("0.02"),
				RegimeType:  tax.RegimeDiferenciado60,
			},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate retornou erro inesperado: %v", err)
	}

	// Alíquota efetiva: 0.010 * 0.4 = 0.004 → 10000 * 0.004 = 40.00
	assertEqual(t, "Projected.GrossTax", "40.00", result.Projected.GrossTax)
	assertEqual(t, "Projected.Credits", "0", result.Projected.Credits)
	assertEqual(t, "Projected.NetTax", "40.00", result.Projected.NetTax)

	// Delta: regime padrão seria 100.00, diferenciado paga 40.00 → economia de 60.00
	// Regime atual: PIS+COFINS = 10000*0.0925=925 + ISS=10000*0.02=200 = 1125.00
	assertEqual(t, "Current.GrossTax", "1125.00", result.Current.GrossTax)
	assertEqual(t, "Delta", "1085.00", result.Delta) // 1125 - 40
}

// TestCalculate_RegimeReduzidoZero valida que serviços da cesta básica pagam zero CBS/IBS.
//
// Ano 2026 — alíquota efetiva = 0.
// Projetado bruto = 0 | Líquido projetado = 0.
func TestCalculate_RegimeReduzidoZero(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{
				ID:          "svc-cesta",
				Description: "Arroz e feijão",
				Amount:      mustDecimal("5000.00"),
				ISSRate:     mustDecimal("0.00"),
				RegimeType:  tax.RegimeReduzidoZero,
			},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate retornou erro inesperado: %v", err)
	}

	assertEqual(t, "Projected.GrossTax", "0", result.Projected.GrossTax)
	assertEqual(t, "Projected.NetTax", "0", result.Projected.NetTax)
}

// TestCalculate_MixedRegimes valida mix de serviços com regimes diferentes.
//
// Clínica com serviços de saúde (diferenciado_60) e exames estéticos (padrao).
// Ano 2026 — CBS+IBS = 0.010
//
//   - Saúde R$ 8.000 × 0.004 = 32.00
//   - Estética R$ 2.000 × 0.010 = 20.00
//
// Projetado bruto = 52.00
func TestCalculate_MixedRegimes(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{
				ID:         "svc-saude",
				Amount:     mustDecimal("8000.00"),
				ISSRate:    mustDecimal("0.02"),
				RegimeType: tax.RegimeDiferenciado60,
			},
			{
				ID:         "svc-estetica",
				Amount:     mustDecimal("2000.00"),
				ISSRate:    mustDecimal("0.05"),
				RegimeType: tax.RegimePadrao,
			},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate retornou erro inesperado: %v", err)
	}

	// Projetado: 8000*0.004 + 2000*0.010 = 32.00 + 20.00 = 52.00
	assertEqual(t, "Projected.GrossTax", "52.00", result.Projected.GrossTax)

	// Atual: PIS+COFINS sobre (8000+2000)*0.0925=925 + ISS=(8000*0.02)+(2000*0.05)=160+100=260
	// Total atual bruto = 925 + 260 = 1185.00
	assertEqual(t, "Current.GrossTax", "1185.00", result.Current.GrossTax)
}

// assertEqual compara o valor de um decimal.Decimal com uma string esperada.
func assertEqual(t *testing.T, label, expected string, got decimal.Decimal) {
	t.Helper()
	exp := mustDecimal(expected)
	if !exp.Equal(got) {
		t.Errorf("%s: esperado %s, obtido %s", label, exp.String(), got.String())
	}
}
