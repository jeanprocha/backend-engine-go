package tax_test

import (
	"context"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
)

// TestCalculate_YearForaDoIntervalo garante que Calculate rejeita ano fora de
// 2026-2033 em vez de silenciosamente clampar e devolver um year que não
// corresponde às regras usadas (W7/B2.2) — o handler HTTP já validava isso
// na borda; esta é a defesa em profundidade para qualquer chamador direto.
func TestCalculate_YearForaDoIntervalo(t *testing.T) {
	calc := tax.NewCalculator()
	svc := []tax.Service{{ID: "svc-1", Amount: mustDecimal("1000.00"), ISSRate: mustDecimal("0.05")}}

	for _, year := range []int{2025, 2050, 0, -1} {
		_, err := calc.Calculate(context.Background(), tax.SimulationInput{Year: year, Services: svc})
		if err == nil {
			t.Errorf("year=%d: esperava erro, Calculate aceitou silenciosamente", year)
		}
	}
}

// TestCalculate_CompanyRegimeDesconhecido garante que um company_regime que
// não corresponde a nenhum perfil reconhecido devolve erro em vez de cair
// silenciosamente no ramo "regular".
func TestCalculate_CompanyRegimeDesconhecido(t *testing.T) {
	calc := tax.NewCalculator()
	svc := []tax.Service{{ID: "svc-1", Amount: mustDecimal("1000.00"), ISSRate: mustDecimal("0.05")}}

	_, err := calc.Calculate(context.Background(), tax.SimulationInput{
		Year: 2026, CompanyRegime: "lucro_presumido", Services: svc,
	})
	if err == nil {
		t.Fatal("company_regime desconhecido: esperava erro, Calculate aceitou silenciosamente")
	}
}

// TestCalculate_CompanyRegimeVazioOuRegularSaoValidos garante que a validação
// nova não regride os dois valores mais comuns (vazio e "regular").
func TestCalculate_CompanyRegimeVazioOuRegularSaoValidos(t *testing.T) {
	calc := tax.NewCalculator()
	svc := []tax.Service{{ID: "svc-1", Amount: mustDecimal("1000.00"), ISSRate: mustDecimal("0.05")}}

	for _, regime := range []string{"", tax.CompanyRegimeRegular} {
		_, err := calc.Calculate(context.Background(), tax.SimulationInput{
			Year: 2026, CompanyRegime: regime, Services: svc,
		})
		if err != nil {
			t.Errorf("company_regime=%q: erro inesperado: %v", regime, err)
		}
	}
}

// TestCalculate_ReprodutivelIndependenteDeEnv prova a reprodutibilidade
// (W7/B2.2): os 5 parâmetros fiscais que antes liam variável de ambiente a
// cada chamada agora são constantes de pacote — setar as env vars com
// valores absurdos não pode mudar o resultado. Pré-requisito para a suíte
// cruzada contra a Calculadora RFB (B2.1): um golden gerado numa máquina
// precisa reproduzir idêntico em qualquer outra.
func TestCalculate_ReprodutivelIndependenteDeEnv(t *testing.T) {
	input := func() tax.SimulationInput {
		return tax.SimulationInput{
			Year:          2026,
			CompanyRegime: tax.CompanyRegimeSimplesPuro,
			Services: []tax.Service{
				{ID: "svc-1", Amount: mustDecimal("10000.00"), ISSRate: mustDecimal("0.05")},
			},
			Expenses: []tax.Expense{
				{ID: "exp-1", Amount: mustDecimal("5000.00"), IsEligible: true},
			},
		}
	}

	calc := tax.NewCalculator()
	before, err := calc.Calculate(context.Background(), input())
	if err != nil {
		t.Fatalf("Calculate (antes): %v", err)
	}

	t.Setenv("SIMPLES_ILLUSTRATIVE_CURRENT_RATE", "0.99")
	t.Setenv("SIMPLES_PURO_EFFECTIVE_IBS_CBS", "0.99")
	t.Setenv("MEI_MONTHLY_DAS_BRL", "999999.99")
	t.Setenv("IMOBILIARIO_REDUTOR_VENDA_BRL", "999999.99")
	t.Setenv("IMOBILIARIO_REDUTOR_ALUGUEL_BRL", "999999.99")

	after, err := calc.Calculate(context.Background(), input())
	if err != nil {
		t.Fatalf("Calculate (depois de setar env absurda): %v", err)
	}

	if !before.Current.NetTax.Equal(after.Current.NetTax) ||
		!before.Projected.NetTax.Equal(after.Projected.NetTax) ||
		!before.Delta.Equal(after.Delta) {
		t.Fatalf("resultado mudou com env var absurda: antes=%+v depois=%+v", before, after)
	}
}
