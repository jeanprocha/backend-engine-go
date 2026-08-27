package tax_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
	"github.com/shopspring/decimal"
)

// canonicalCase espelha testdata/casos_canonicos.json — reusado pela suíte
// cruzada contra a Calculadora RFB (W7/B2.1) quando ela existir.
type canonicalCase struct {
	Description   string         `json:"description"`
	CompanyRegime string         `json:"company_regime"`
	Services      []canonicalSvc `json:"services"`
	Expenses      []canonicalExp `json:"expenses"`
}

type canonicalSvc struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Amount      string `json:"amount"`
	ISSRate     string `json:"iss_rate"`
	RegimeType  string `json:"regime_type"`
}

type canonicalExp struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Amount      string `json:"amount"`
	IsEligible  bool   `json:"is_eligible"`
	RegimeType  string `json:"regime_type"`
}

func loadCanonicalCases(t *testing.T) map[string]canonicalCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "casos_canonicos.json"))
	if err != nil {
		t.Fatalf("lendo testdata/casos_canonicos.json: %v", err)
	}
	var cases map[string]canonicalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parseando testdata/casos_canonicos.json: %v", err)
	}
	return cases
}

func (c canonicalCase) toInput(year int) tax.SimulationInput {
	services := make([]tax.Service, len(c.Services))
	for i, s := range c.Services {
		services[i] = tax.Service{
			ID:          s.ID,
			Description: s.Description,
			Amount:      decimal.RequireFromString(s.Amount),
			ISSRate:     decimal.RequireFromString(s.ISSRate),
			RegimeType:  s.RegimeType,
		}
	}
	expenses := make([]tax.Expense, len(c.Expenses))
	for i, e := range c.Expenses {
		expenses[i] = tax.Expense{
			ID:          e.ID,
			Description: e.Description,
			Amount:      decimal.RequireFromString(e.Amount),
			IsEligible:  e.IsEligible,
			RegimeType:  e.RegimeType,
		}
	}
	return tax.SimulationInput{
		Year:          year,
		CompanyRegime: c.CompanyRegime,
		Services:      services,
		Expenses:      expenses,
	}
}

// TestRulesForYear_TodosOsAnos trava, para cada ano de 2026 a 2033, os quatro
// valores que RulesForYear devolve mais o fator ISS municipal.
//
// Valores corrigidos contra o calendário legal (W7/B2.2 —
// docs/roadmap-execucao.md): PIS/COFINS extintos a partir de 2027 (não
// 70%/40% em 2027/2028); CBS entra plena em 2027 (~8,7% = referência menos
// redução compensatória de 0,1 p.p.) com IBS ainda nominal em 0,1%; a rampa
// do IBS/ICMS/ISS 2029-2032 é de 1/10 ao ano (não 1/5); o split de 2033 é
// 8,8%/17,7% (projeção oficial MF/TCU), não 9,9%/16,6%. Ver RuleBasis em
// transition_table.go para a proveniência de cada valor.
func TestRulesForYear_TodosOsAnos(t *testing.T) {
	cases := []struct {
		year            int
		pisCofinsFactor string
		cbsRate         string
		ibsRate         string
		issFactor       string
	}{
		{2026, "1", "0.009", "0.001", "1"},
		{2027, "0", "0.087", "0.001", "1"},
		{2028, "0", "0.087", "0.001", "1"},
		{2029, "0", "0.088", "0.0177", "0.9"},
		{2030, "0", "0.088", "0.0354", "0.8"},
		{2031, "0", "0.088", "0.0531", "0.7"},
		{2032, "0", "0.088", "0.0708", "0.6"},
		{2033, "0", "0.088", "0.177", "0"},
	}
	for _, tc := range cases {
		t.Run(intToStr(tc.year), func(t *testing.T) {
			r := tax.RulesForYear(tc.year)
			assertDecimalEqual(t, "PISCOFINSFactor", tc.pisCofinsFactor, r.PISCOFINSFactor)
			assertDecimalEqual(t, "CBSRate", tc.cbsRate, r.CBSRate)
			assertDecimalEqual(t, "IBSRate", tc.ibsRate, r.IBSRate)
			assertDecimalEqual(t, "ISSMunicipalTransitionFactor", tc.issFactor, r.ISSMunicipalTransitionFactor())
		})
	}
}

// TestCalculate_EmpresaServicosPadrao_TodosOsAnos trava o resultado completo
// (bruto, créditos, líquido, delta) do caso canônico "empresa_servicos_padrao"
// para cada ano de 2026 a 2033.
//
// Valores corrigidos contra o calendário legal (W7/B2.2 — ver o comentário de
// TestRulesForYear_TodosOsAnos). 2026 e 2033 não mudam de valor agregado
// nesta tabela porque o caso canônico usa regime "regular" (a alíquota
// projetada é sempre CBS+IBS combinado — só a composição interna muda; ver
// TestTaxComponents_SomaReproduzGrossTax para a decomposição). 2027-2032
// mudam: PIS/COFINS extintos derruba o bruto atual; CBS entrando plena em
// 2027 e a rampa de 1/10 (não 1/5) do IBS mudam o bruto projetado — a ponto
// do sinal do delta inverter em alguns anos (a economia projetada vira
// custo adicional), refletindo a lei real, não a estimativa anterior.
func TestCalculate_EmpresaServicosPadrao_TodosOsAnos(t *testing.T) {
	cases := loadCanonicalCases(t)
	c, ok := cases["empresa_servicos_padrao"]
	if !ok {
		t.Fatal("caso canônico \"empresa_servicos_padrao\" ausente de testdata/casos_canonicos.json")
	}

	expected := map[int]struct {
		currentGross, currentCredits, currentNet       string
		projectedGross, projectedCredits, projectedNet string
		delta, deltaPct                                string
	}{
		2026: {"1425.00", "370.00", "1055.00", "100.00", "40.00", "60.00", "-995.00", "-94.31"},
		2027: {"500.00", "0.00", "500.00", "880.00", "352.00", "528.00", "28.00", "5.60"},
		2028: {"500.00", "0.00", "500.00", "880.00", "352.00", "528.00", "28.00", "5.60"},
		2029: {"450.00", "0.00", "450.00", "1057.00", "422.80", "634.20", "184.20", "40.93"},
		2030: {"400.00", "0.00", "400.00", "1234.00", "493.60", "740.40", "340.40", "85.10"},
		2031: {"350.00", "0.00", "350.00", "1411.00", "564.40", "846.60", "496.60", "141.89"},
		2032: {"300.00", "0.00", "300.00", "1588.00", "635.20", "952.80", "652.80", "217.60"},
		2033: {"0.00", "0.00", "0.00", "2650.00", "1060.00", "1590.00", "1590.00", "0.00"},
	}

	calc := tax.NewCalculator()
	for year, want := range expected {
		t.Run(intToStr(year), func(t *testing.T) {
			result, err := calc.Calculate(context.Background(), c.toInput(year))
			if err != nil {
				t.Fatalf("Calculate(%d): erro inesperado: %v", year, err)
			}
			assertDecimalEqual(t, "Current.GrossTax", want.currentGross, result.Current.GrossTax)
			assertDecimalEqual(t, "Current.Credits", want.currentCredits, result.Current.Credits)
			assertDecimalEqual(t, "Current.NetTax", want.currentNet, result.Current.NetTax)
			assertDecimalEqual(t, "Projected.GrossTax", want.projectedGross, result.Projected.GrossTax)
			assertDecimalEqual(t, "Projected.Credits", want.projectedCredits, result.Projected.Credits)
			assertDecimalEqual(t, "Projected.NetTax", want.projectedNet, result.Projected.NetTax)
			assertDecimalEqual(t, "Delta", want.delta, result.Delta)
			assertDecimalEqual(t, "DeltaPct", want.deltaPct, result.DeltaPct)
		})
	}
}

func assertDecimalEqual(t *testing.T, field, want string, got decimal.Decimal) {
	t.Helper()
	w := decimal.RequireFromString(want)
	if !got.Equal(w) {
		t.Errorf("%s: got %s want %s", field, got.String(), w.String())
	}
}

func intToStr(y int) string {
	return decimal.NewFromInt(int64(y)).String()
}
