package tax_test

import (
	"context"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
	"github.com/shopspring/decimal"
)

// TestTaxComponents_SomaReproduzGrossTax verifica a invariante de
// TaxComponents (W7/B2.1): a soma dos tributos decompostos reproduz o
// GrossTax agregado, com tolerância de 1 centavo por arredondamento
// independente (ver o comentário de TaxComponents em entity.go). Cobre os
// regimes que passam por computeCurrentRegularLegacy (todos, exceto MEI e
// Simples) e por computeProjectedCBSIBSForcedOutputRegime (regular,
// diferenciado_60, prof_liberal, aliquota_zero, exportadora, entidade_imune).
//
// MEI, Simples e imobiliário não decompõem em PIS/COFINS/CBS/IBS — usam
// alíquota única ilustrativa, fora da taxonomia da Calculadora RFB — e por
// isso ficam de fora deste teste; Components permanece zero-value nesses
// perfis, o que já é o comportamento correto (ver entity.go).
func TestTaxComponents_SomaReproduzGrossTax(t *testing.T) {
	cases := loadCanonicalCases(t)
	c, ok := cases["empresa_servicos_padrao"]
	if !ok {
		t.Fatal("caso canônico \"empresa_servicos_padrao\" ausente de testdata/casos_canonicos.json")
	}

	tolerance := decimal.RequireFromString("0.01")
	regimes := []string{
		"", // regular
		tax.CompanyRegimeSectorDiferenciado60,
		tax.CompanyRegimeProfissionalLiberal,
		tax.CompanyRegimeAliquotaZero,
		tax.CompanyRegimeExportadora,
		tax.CompanyRegimeEntidadeImune,
	}

	calc := tax.NewCalculator()
	for _, regime := range regimes {
		for year := 2026; year <= 2033; year++ {
			input := c.toInput(year)
			input.CompanyRegime = regime
			result, err := calc.Calculate(context.Background(), input)
			if err != nil {
				t.Fatalf("regime=%q year=%d: Calculate: %v", regime, year, err)
			}

			currentSum := result.Current.Components.PIS.
				Add(result.Current.Components.COFINS).
				Add(result.Current.Components.ISS)
			currentDiff := currentSum.Sub(result.Current.GrossTax).Abs()
			if currentDiff.GreaterThan(tolerance) {
				t.Errorf("regime=%q year=%d: Current.Components soma %s, GrossTax %s (diff %s > %s)",
					regime, year, currentSum, result.Current.GrossTax, currentDiff, tolerance)
			}

			projectedSum := result.Projected.Components.CBS.Add(result.Projected.Components.IBS)
			projectedDiff := projectedSum.Sub(result.Projected.GrossTax).Abs()
			if projectedDiff.GreaterThan(tolerance) {
				t.Errorf("regime=%q year=%d: Projected.Components soma %s, GrossTax %s (diff %s > %s)",
					regime, year, projectedSum, result.Projected.GrossTax, projectedDiff, tolerance)
			}
		}
	}
}

// TestTaxComponents_MEISemDecomposicao documenta que MEI não decompõe por
// tributo — Components fica zero-value nos dois lados, de propósito.
func TestTaxComponents_MEISemDecomposicao(t *testing.T) {
	calc := tax.NewCalculator()
	input := tax.SimulationInput{
		Year:          2030,
		CompanyRegime: tax.CompanyRegimeMEI,
		Services: []tax.Service{
			{ID: "svc-1", Amount: mustDecimal("6000.00"), ISSRate: mustDecimal("0.05")},
		},
	}
	result, err := calc.Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if !result.Current.Components.Sum().IsZero() || !result.Projected.Components.Sum().IsZero() {
		t.Errorf("MEI deveria ter Components zero-value; got current=%v projected=%v",
			result.Current.Components, result.Projected.Components)
	}
}
