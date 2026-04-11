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
//
//	PIS+COFINS = 10000 * (0.0165 + 0.076) = 10000 * 0.0925 = 925.00
//	ISS        = 10000 * 0.05 = 500.00
//	Bruto atual = 1425.00 | Créditos = 0 | Líquido atual = 1425.00
//
// Esperado regime projetado:
//
//	CBS+IBS = 10000 * (0.009 + 0.001) = 10000 * 0.010 = 100.00
//	Créditos = 0 | Líquido projetado = 100.00
//
// Delta = 100.00 - 1425.00 = -1325.00 (economia; convencao projetado − atual)
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

	assertEqual(t, "Delta", "-1325.00", result.Delta)
}

// TestCalculate_AllEligibleExpenses valida créditos integrais sobre despesas elegíveis.
// Receita: R$ 10.000,00 | Despesas elegíveis: R$ 5.000,00.
//
// Regime projetado (2026):
//
//	Bruto = 10000 * 0.010 = 100.00
//	Créditos = 5000 * 0.010 = 50.00
//	Líquido projetado = 50.00
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

// TestCalculate_GoldenFractionalMoney valida arredondamento com receita e despesa com centavos "quebrados"
// (cifrão de ouro). Regime regular, 2026: CBS+IBS combinado 1% na projeção.
//
//	Bruto projetado = 1234567.89 × 0.01 → 12345.68
//	Créditos = 100000.33 × 0.01 → 1000.00
//	Líquido projetado = 11345.68
func TestCalculate_GoldenFractionalMoney(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "svc-1", Amount: mustDecimal("1234567.89"), ISSRate: mustDecimal("0.05")},
		},
		Expenses: []tax.Expense{
			{ID: "exp-1", Amount: mustDecimal("100000.33"), IsEligible: true},
		},
	}
	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	assertEqual(t, "Projected.GrossTax", "12345.68", result.Projected.GrossTax)
	assertEqual(t, "Projected.Credits", "1000.00", result.Projected.Credits)
	assertEqual(t, "Projected.NetTax", "11345.68", result.Projected.NetTax)
}

// TestCalculate_MEIProfile usa carga fixa mensal (DAS ilustrativo) nos dois cenários; delta ~ 0.
func TestCalculate_MEIProfile(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year:          2030,
		CompanyRegime: tax.CompanyRegimeMEI,
		Services: []tax.Service{
			{ID: "svc-1", Amount: mustDecimal("6000.00"), ISSRate: mustDecimal("0.05")},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate retornou erro inesperado: %v", err)
	}

	assertEqual(t, "Current.NetTax", "85.00", result.Current.NetTax)
	assertEqual(t, "Projected.NetTax", "85.00", result.Projected.NetTax)
	assertEqual(t, "Delta", "0", result.Delta)
}

// TestCalculate_ContextMentioningMEIWithoutRegime garante que o texto livre não ativa o ramo MEI;
// só `company_regime: mei` aplica DAS ilustrativo.
func TestCalculate_ContextMentioningMEIWithoutRegime(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year:           2030,
		CompanyContext: "Sou MEI de desenvolvimento",
		Services: []tax.Service{
			{ID: "svc-1", Amount: mustDecimal("6000.00"), ISSRate: mustDecimal("0.05")},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate retornou erro inesperado: %v", err)
	}

	// 2030: ISS municipal com factor 0,6 sobre a alíquota informada (transição LC 68 — premissa TribIA).
	assertEqual(t, "Current.NetTax", "263.25", result.Current.NetTax)
	assertEqual(t, "Projected.NetTax", "1200", result.Projected.NetTax)
	assertEqual(t, "Delta", "936.75", result.Delta)
}

// TestCalculate_SimplesPuro usa baseline ilustrativo no atual e alíquota baixa sem créditos no projetado.
func TestCalculate_SimplesPuro(t *testing.T) {
	t.Setenv("SIMPLES_ILLUSTRATIVE_CURRENT_RATE", "0.06")
	t.Setenv("SIMPLES_PURO_EFFECTIVE_IBS_CBS", "0.04")
	calc := newCalc()
	input := tax.SimulationInput{
		Year:          2026,
		CompanyRegime: tax.CompanyRegimeSimplesPuro,
		Services: []tax.Service{
			{ID: "svc-1", Amount: mustDecimal("10000.00"), ISSRate: mustDecimal("0.05")},
		},
		Expenses: []tax.Expense{
			{ID: "exp-1", Amount: mustDecimal("5000.00"), IsEligible: true},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate retornou erro inesperado: %v", err)
	}

	assertEqual(t, "Current.NetTax", "600.00", result.Current.NetTax)
	assertEqual(t, "Current.Credits", "0", result.Current.Credits)
	assertEqual(t, "Projected.GrossTax", "400.00", result.Projected.GrossTax)
	assertEqual(t, "Projected.Credits", "0", result.Projected.Credits)
	assertEqual(t, "Projected.NetTax", "400.00", result.Projected.NetTax)
	assertEqual(t, "Delta", "-200.00", result.Delta)
}

// TestCalculate_SimplesHibrido_ProjectedMatchesRegular: projeção CBS/IBS idêntica ao perfil regular.
func TestCalculate_SimplesHibrido_ProjectedMatchesRegular(t *testing.T) {
	t.Setenv("SIMPLES_ILLUSTRATIVE_CURRENT_RATE", "0.06")
	calc := newCalc()
	base := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "svc-1", Amount: mustDecimal("10000.00"), ISSRate: mustDecimal("0.05")},
		},
		Expenses: []tax.Expense{
			{ID: "exp-1", Amount: mustDecimal("5000.00"), IsEligible: true},
		},
	}
	regular := base
	regular.CompanyRegime = tax.CompanyRegimeRegular
	hybrid := base
	hybrid.CompanyRegime = tax.CompanyRegimeSimplesHibrido

	gotReg, err := calc.Calculate(context.Background(), regular)
	if err != nil {
		t.Fatalf("regular: %v", err)
	}
	gotHyb, err := calc.Calculate(context.Background(), hybrid)
	if err != nil {
		t.Fatalf("simples_hibrido: %v", err)
	}

	if !gotReg.Projected.GrossTax.Equal(gotHyb.Projected.GrossTax) {
		t.Errorf("Projected.GrossTax: regular=%s simples_hibrido=%s", gotReg.Projected.GrossTax, gotHyb.Projected.GrossTax)
	}
	if !gotReg.Projected.Credits.Equal(gotHyb.Projected.Credits) {
		t.Errorf("Projected.Credits: regular=%s simples_hibrido=%s", gotReg.Projected.Credits, gotHyb.Projected.Credits)
	}
	if !gotReg.Projected.NetTax.Equal(gotHyb.Projected.NetTax) {
		t.Errorf("Projected.NetTax: regular=%s simples_hibrido=%s", gotReg.Projected.NetTax, gotHyb.Projected.NetTax)
	}
	assertEqual(t, "hybrid.Current.NetTax", "600.00", gotHyb.Current.NetTax)
}

// TestCalculate_SimplesPuroAndHibrido_SharedCurrent: mesmo faturamento → mesmo "atual" ilustrativo.
func TestCalculate_SimplesPuroAndHibrido_SharedCurrent(t *testing.T) {
	t.Setenv("SIMPLES_ILLUSTRATIVE_CURRENT_RATE", "0.06")
	t.Setenv("SIMPLES_PURO_EFFECTIVE_IBS_CBS", "0.04")
	calc := newCalc()
	svc := []tax.Service{{ID: "svc-1", Amount: mustDecimal("10000.00"), ISSRate: mustDecimal("0.05")}}
	puro, _ := calc.Calculate(context.Background(), tax.SimulationInput{
		Year: 2026, CompanyRegime: tax.CompanyRegimeSimplesPuro, Services: svc,
	})
	hyb, _ := calc.Calculate(context.Background(), tax.SimulationInput{
		Year: 2026, CompanyRegime: tax.CompanyRegimeSimplesHibrido, Services: svc,
	})
	if !puro.Current.NetTax.Equal(hyb.Current.NetTax) {
		t.Errorf("Current divergiu: puro=%s hibrido=%s", puro.Current.NetTax, hyb.Current.NetTax)
	}
}

// TestCalculate_CompanyProfileDiferenciado60_CurrentMatchesRegular: atual idêntico ao perfil regular.
func TestCalculate_CompanyProfileDiferenciado60_CurrentMatchesRegular(t *testing.T) {
	calc := newCalc()
	base := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "a", Amount: mustDecimal("8000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimeDiferenciado60},
			{ID: "b", Amount: mustDecimal("2000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{
			{ID: "e1", Amount: mustDecimal("1000.00"), IsEligible: true},
		},
	}
	reg := base
	reg.CompanyRegime = tax.CompanyRegimeRegular
	sec := base
	sec.CompanyRegime = tax.CompanyRegimeSectorDiferenciado60

	gotReg, err := calc.Calculate(context.Background(), reg)
	if err != nil {
		t.Fatalf("regular: %v", err)
	}
	gotSec, err := calc.Calculate(context.Background(), sec)
	if err != nil {
		t.Fatalf("diferenciado_60 profile: %v", err)
	}
	if !gotReg.Current.GrossTax.Equal(gotSec.Current.GrossTax) {
		t.Errorf("Current.GrossTax: regular=%s profile=%s", gotReg.Current.GrossTax, gotSec.Current.GrossTax)
	}
	if !gotReg.Current.Credits.Equal(gotSec.Current.Credits) {
		t.Errorf("Current.Credits: regular=%s profile=%s", gotReg.Current.Credits, gotSec.Current.Credits)
	}
	if !gotReg.Current.NetTax.Equal(gotSec.Current.NetTax) {
		t.Errorf("Current.NetTax: regular=%s profile=%s", gotReg.Current.NetTax, gotSec.Current.NetTax)
	}
}

// TestCalculate_CompanyProfileDiferenciado60_ForcesReducedRateOnAllServices ignora regime_type divergente na saída.
func TestCalculate_CompanyProfileDiferenciado60_ForcesReducedRateOnAllServices(t *testing.T) {
	calc := newCalc()
	// Mix: saúde (diferenciado) + estética (padrão). Sem perfil: 52.00 de bruto projetado em 2026.
	input := tax.SimulationInput{
		Year:          2026,
		CompanyRegime: tax.CompanyRegimeSectorDiferenciado60,
		Services: []tax.Service{
			{ID: "svc-saude", Amount: mustDecimal("8000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimeDiferenciado60},
			{ID: "svc-estetica", Amount: mustDecimal("2000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	// Ambos os serviços com alíquota efetiva 0.004: 10000 * 0.004 = 40.00
	assertEqual(t, "Projected.GrossTax", "40.00", result.Projected.GrossTax)
	assertEqual(t, "Current.GrossTax", "1185.00", result.Current.GrossTax)
}

// TestCalculate_CompanyProfileProfissionalLiberal_CurrentMatchesRegular: atual idêntico ao perfil regular.
func TestCalculate_CompanyProfileProfissionalLiberal_CurrentMatchesRegular(t *testing.T) {
	calc := newCalc()
	base := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "a", Amount: mustDecimal("8000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimePadrao},
			{ID: "b", Amount: mustDecimal("2000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{
			{ID: "e1", Amount: mustDecimal("1000.00"), IsEligible: true},
		},
	}
	reg := base
	reg.CompanyRegime = tax.CompanyRegimeRegular
	pl := base
	pl.CompanyRegime = tax.CompanyRegimeProfissionalLiberal

	gotReg, err := calc.Calculate(context.Background(), reg)
	if err != nil {
		t.Fatalf("regular: %v", err)
	}
	gotPl, err := calc.Calculate(context.Background(), pl)
	if err != nil {
		t.Fatalf("prof_liberal profile: %v", err)
	}
	if !gotReg.Current.GrossTax.Equal(gotPl.Current.GrossTax) {
		t.Errorf("Current.GrossTax: regular=%s profile=%s", gotReg.Current.GrossTax, gotPl.Current.GrossTax)
	}
	if !gotReg.Current.Credits.Equal(gotPl.Current.Credits) {
		t.Errorf("Current.Credits: regular=%s profile=%s", gotReg.Current.Credits, gotPl.Current.Credits)
	}
	if !gotReg.Current.NetTax.Equal(gotPl.Current.NetTax) {
		t.Errorf("Current.NetTax: regular=%s profile=%s", gotReg.Current.NetTax, gotPl.Current.NetTax)
	}
}

// TestCalculate_CompanyProfileProfissionalLiberal_Forces70PctRateOnAllServices força 70% da alíquota padrão na saída (2026: 1% × 0,7).
func TestCalculate_CompanyProfileProfissionalLiberal_Forces70PctRateOnAllServices(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year:          2026,
		CompanyRegime: tax.CompanyRegimeProfissionalLiberal,
		Services: []tax.Service{
			{ID: "svc-a", Amount: mustDecimal("8000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimeDiferenciado60},
			{ID: "svc-b", Amount: mustDecimal("2000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
	}

	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	// 10000 * 0.01 * 0.7 = 70.00
	assertEqual(t, "Projected.GrossTax", "70.00", result.Projected.GrossTax)
	assertEqual(t, "Current.GrossTax", "1185.00", result.Current.GrossTax)
}

// TestCalculate_CompanyProfileAliquotaZero_CurrentMatchesRegular: atual idêntico ao perfil regular.
func TestCalculate_CompanyProfileAliquotaZero_CurrentMatchesRegular(t *testing.T) {
	calc := newCalc()
	base := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "a", Amount: mustDecimal("5000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimeReduzidoZero},
			{ID: "b", Amount: mustDecimal("5000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{
			{ID: "e1", Amount: mustDecimal("1000.00"), IsEligible: true},
		},
	}
	reg := base
	reg.CompanyRegime = tax.CompanyRegimeRegular
	az := base
	az.CompanyRegime = tax.CompanyRegimeAliquotaZero

	gotReg, err := calc.Calculate(context.Background(), reg)
	if err != nil {
		t.Fatalf("regular: %v", err)
	}
	gotAz, err := calc.Calculate(context.Background(), az)
	if err != nil {
		t.Fatalf("aliquota_zero profile: %v", err)
	}
	if !gotReg.Current.GrossTax.Equal(gotAz.Current.GrossTax) {
		t.Errorf("Current.GrossTax: regular=%s profile=%s", gotReg.Current.GrossTax, gotAz.Current.GrossTax)
	}
	if !gotReg.Current.Credits.Equal(gotAz.Current.Credits) {
		t.Errorf("Current.Credits: regular=%s profile=%s", gotReg.Current.Credits, gotAz.Current.Credits)
	}
	if !gotReg.Current.NetTax.Equal(gotAz.Current.NetTax) {
		t.Errorf("Current.NetTax: regular=%s profile=%s", gotReg.Current.NetTax, gotAz.Current.NetTax)
	}
}

// TestCalculate_CompanyProfileAliquotaZero_ForcesZeroOnAllServices: bruto projetado zero; líquido negativo com créditos.
func TestCalculate_CompanyProfileAliquotaZero_ForcesZeroOnAllServices(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year:          2026,
		CompanyRegime: tax.CompanyRegimeAliquotaZero,
		Services: []tax.Service{
			{ID: "cesta", Amount: mustDecimal("8000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimeReduzidoZero},
			{ID: "luxo", Amount: mustDecimal("2000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{
			{ID: "inp", Amount: mustDecimal("1000.00"), IsEligible: true, RegimeType: tax.RegimePadrao},
		},
	}
	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	assertEqual(t, "Projected.GrossTax", "0", result.Projected.GrossTax)
	// 2026 CBS+IBS padrão = 0.01 → crédito 1000 * 0.01 = 10
	assertEqual(t, "Projected.Credits", "10.00", result.Projected.Credits)
	assertEqual(t, "Projected.NetTax", "-10.00", result.Projected.NetTax)
}

// TestCalculate_CompanyProfileExportadora_CurrentMatchesRegular: atual idêntico ao perfil regular.
func TestCalculate_CompanyProfileExportadora_CurrentMatchesRegular(t *testing.T) {
	calc := newCalc()
	base := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "a", Amount: mustDecimal("5000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimeReduzidoZero},
			{ID: "b", Amount: mustDecimal("5000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{
			{ID: "e1", Amount: mustDecimal("1000.00"), IsEligible: true},
		},
	}
	reg := base
	reg.CompanyRegime = tax.CompanyRegimeRegular
	ex := base
	ex.CompanyRegime = tax.CompanyRegimeExportadora

	gotReg, err := calc.Calculate(context.Background(), reg)
	if err != nil {
		t.Fatalf("regular: %v", err)
	}
	gotEx, err := calc.Calculate(context.Background(), ex)
	if err != nil {
		t.Fatalf("exportadora profile: %v", err)
	}
	if !gotReg.Current.GrossTax.Equal(gotEx.Current.GrossTax) {
		t.Errorf("Current.GrossTax: regular=%s profile=%s", gotReg.Current.GrossTax, gotEx.Current.GrossTax)
	}
	if !gotReg.Current.Credits.Equal(gotEx.Current.Credits) {
		t.Errorf("Current.Credits: regular=%s profile=%s", gotReg.Current.Credits, gotEx.Current.Credits)
	}
	if !gotReg.Current.NetTax.Equal(gotEx.Current.NetTax) {
		t.Errorf("Current.NetTax: regular=%s profile=%s", gotReg.Current.NetTax, gotEx.Current.NetTax)
	}
}

// TestCalculate_CompanyProfileExportadora_ForcesZeroOnAllServices: bruto projetado zero; líquido negativo com créditos.
func TestCalculate_CompanyProfileExportadora_ForcesZeroOnAllServices(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year:          2026,
		CompanyRegime: tax.CompanyRegimeExportadora,
		Services: []tax.Service{
			{ID: "exp", Amount: mustDecimal("8000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimePadrao},
			{ID: "exp2", Amount: mustDecimal("2000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{
			{ID: "inp", Amount: mustDecimal("1000.00"), IsEligible: true, RegimeType: tax.RegimePadrao},
		},
	}
	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	assertEqual(t, "Projected.GrossTax", "0", result.Projected.GrossTax)
	assertEqual(t, "Projected.Credits", "10.00", result.Projected.Credits)
	assertEqual(t, "Projected.NetTax", "-10.00", result.Projected.NetTax)
}

// TestCalculate_CompanyProfileEntidadeImune_CurrentMatchesRegular: atual idêntico ao perfil regular.
func TestCalculate_CompanyProfileEntidadeImune_CurrentMatchesRegular(t *testing.T) {
	calc := newCalc()
	base := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "a", Amount: mustDecimal("5000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimePadrao},
			{ID: "b", Amount: mustDecimal("5000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{
			{ID: "e1", Amount: mustDecimal("1000.00"), IsEligible: true},
		},
	}
	reg := base
	reg.CompanyRegime = tax.CompanyRegimeRegular
	im := base
	im.CompanyRegime = tax.CompanyRegimeEntidadeImune

	gotReg, err := calc.Calculate(context.Background(), reg)
	if err != nil {
		t.Fatalf("regular: %v", err)
	}
	gotIm, err := calc.Calculate(context.Background(), im)
	if err != nil {
		t.Fatalf("entidade_imune profile: %v", err)
	}
	if !gotReg.Current.GrossTax.Equal(gotIm.Current.GrossTax) {
		t.Errorf("Current.GrossTax: regular=%s profile=%s", gotReg.Current.GrossTax, gotIm.Current.GrossTax)
	}
	if !gotReg.Current.Credits.Equal(gotIm.Current.Credits) {
		t.Errorf("Current.Credits: regular=%s profile=%s", gotReg.Current.Credits, gotIm.Current.Credits)
	}
	if !gotReg.Current.NetTax.Equal(gotIm.Current.NetTax) {
		t.Errorf("Current.NetTax: regular=%s profile=%s", gotReg.Current.NetTax, gotIm.Current.NetTax)
	}
}

// TestCalculate_CompanyProfileEntidadeImune_ProjectedNoCredits: bruto projetado zero; créditos zerados apesar de despesas elegíveis.
func TestCalculate_CompanyProfileEntidadeImune_ProjectedNoCredits(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year:          2026,
		CompanyRegime: tax.CompanyRegimeEntidadeImune,
		Services: []tax.Service{
			{ID: "s1", Amount: mustDecimal("8000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{
			{ID: "inp", Amount: mustDecimal("1000.00"), IsEligible: true, RegimeType: tax.RegimePadrao},
		},
	}
	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	assertEqual(t, "Projected.GrossTax", "0", result.Projected.GrossTax)
	assertEqual(t, "Projected.Credits", "0", result.Projected.Credits)
	assertEqual(t, "Projected.NetTax", "0", result.Projected.NetTax)
}

// TestCalculate_CompanyProfileImobiliario_CurrentMatchesRegular: atual idêntico ao perfil regular.
func TestCalculate_CompanyProfileImobiliario_CurrentMatchesRegular(t *testing.T) {
	calc := newCalc()
	base := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "a", Amount: mustDecimal("6000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimePadrao},
			{ID: "b", Amount: mustDecimal("4000.00"), ISSRate: mustDecimal("0.05"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{
			{ID: "e1", Amount: mustDecimal("500.00"), IsEligible: true},
		},
	}
	reg := base
	reg.CompanyRegime = tax.CompanyRegimeRegular
	im := base
	im.CompanyRegime = tax.CompanyRegimeImobiliarioVenda

	gotReg, err := calc.Calculate(context.Background(), reg)
	if err != nil {
		t.Fatalf("regular: %v", err)
	}
	gotIm, err := calc.Calculate(context.Background(), im)
	if err != nil {
		t.Fatalf("imobiliario_venda: %v", err)
	}
	if !gotReg.Current.GrossTax.Equal(gotIm.Current.GrossTax) {
		t.Errorf("Current.GrossTax: regular=%s profile=%s", gotReg.Current.GrossTax, gotIm.Current.GrossTax)
	}
	if !gotReg.Current.Credits.Equal(gotIm.Current.Credits) {
		t.Errorf("Current.Credits: regular=%s profile=%s", gotReg.Current.Credits, gotIm.Current.Credits)
	}
	if !gotReg.Current.NetTax.Equal(gotIm.Current.NetTax) {
		t.Errorf("Current.NetTax: regular=%s profile=%s", gotReg.Current.NetTax, gotIm.Current.NetTax)
	}
}

// TestCalculate_CompanyProfileImobiliarioVenda_2033: base × 0,265 × 0,6 sem redutor.
func TestCalculate_CompanyProfileImobiliarioVenda_2033(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year:          2033,
		CompanyRegime: tax.CompanyRegimeImobiliarioVenda,
		Services: []tax.Service{
			{ID: "u", Amount: mustDecimal("1000000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimePadrao},
		},
	}
	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	// 1_000_000 * 0.265 * 0.6 = 159000.00
	assertEqual(t, "Projected.GrossTax", "159000.00", result.Projected.GrossTax)
}

// TestCalculate_CompanyProfileImobiliarioAluguel_2033: multiplicador 0,4.
func TestCalculate_CompanyProfileImobiliarioAluguel_2033(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year:          2033,
		CompanyRegime: tax.CompanyRegimeImobiliarioAluguel,
		Services: []tax.Service{
			{ID: "u", Amount: mustDecimal("1000000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimePadrao},
		},
	}
	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	assertEqual(t, "Projected.GrossTax", "106000.00", result.Projected.GrossTax)
}

// TestCalculate_CompanyProfileImobiliario_RedutorTruncatesBase: receita menor que redutor → bruto projetado 0.
func TestCalculate_CompanyProfileImobiliario_RedutorTruncatesBase(t *testing.T) {
	calc := newCalc()
	input := tax.SimulationInput{
		Year:                        2033,
		CompanyRegime:               tax.CompanyRegimeImobiliarioVenda,
		ImobiliarioRedutorAjusteBRL: mustDecimal("500000.00"),
		Services: []tax.Service{
			{ID: "u", Amount: mustDecimal("400000.00"), ISSRate: mustDecimal("0.02"), RegimeType: tax.RegimePadrao},
		},
	}
	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	assertEqual(t, "Projected.GrossTax", "0", result.Projected.GrossTax)
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

	// Delta = 40 - 1125 = -1085 (economia no líquido projetado vs atual)
	// Regime atual: PIS+COFINS = 10000*0.0925=925 + ISS=10000*0.02=200 = 1125.00
	assertEqual(t, "Current.GrossTax", "1125.00", result.Current.GrossTax)
	assertEqual(t, "Delta", "-1085.00", result.Delta)
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
