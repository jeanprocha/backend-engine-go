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
// valores que RulesForYear devolve mais o fator ISS municipal — hoje 2027,
// 2028, 2029, 2031 e 2032 não têm nenhuma asserção numérica em todo o pacote
// (rules_iss_test.go só cobre 2026/2028/2029/2030/2031/2032/2033 no fator ISS
// e 2026/2033 em CBS/IBS/PISCOFINSFactor).
//
// Os valores travados aqui são os ATUAIS — incluindo os que W7/B2.2 vai provar
// que divergem do calendário legal (ver docs/roadmap-execucao.md). O objetivo
// desta PR não é corrigir; é tornar a próxima correção um diff revisável em
// vez de um deslocamento silencioso.
func TestRulesForYear_TodosOsAnos(t *testing.T) {
	cases := []struct {
		year            int
		pisCofinsFactor string
		cbsRate         string
		ibsRate         string
		issFactor       string
	}{
		{2026, "1", "0.009", "0.001", "1"},
		{2027, "0.7", "0.015", "0.035", "1"},
		{2028, "0.4", "0.030", "0.080", "1"},
		{2029, "0.225", "0.050", "0.115", "0.8"},
		{2030, "0.150", "0.065", "0.135", "0.6"},
		{2031, "0.075", "0.080", "0.150", "0.4"},
		{2032, "0", "0.090", "0.160", "0.2"},
		{2033, "0", "0.099", "0.166", "0"},
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
// para cada ano de 2026 a 2033 — a ponta a ponta que faltava para os cinco
// anos sem asserção.
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
		2027: {"1147.50", "259.00", "888.50", "500.00", "200.00", "300.00", "-588.50", "-66.24"},
		2028: {"870.00", "148.00", "722.00", "1100.00", "440.00", "660.00", "-62.00", "-8.59"},
		2029: {"608.13", "83.25", "524.88", "1650.00", "660.00", "990.00", "465.12", "88.61"},
		2030: {"438.75", "55.50", "383.25", "2000.00", "800.00", "1200.00", "816.75", "213.11"},
		2031: {"269.38", "27.75", "241.63", "2300.00", "920.00", "1380.00", "1138.37", "471.12"},
		2032: {"100.00", "0.00", "100.00", "2500.00", "1000.00", "1500.00", "1400.00", "1400.00"},
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
