package tax

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// Engine define o contrato do motor de simulação tributária.
// Ao depender da interface (e não da struct concreta), a camada HTTP pode
// receber um mock nos testes sem acessar banco ou fazer cálculos reais.
type Engine interface {
	Calculate(ctx context.Context, input SimulationInput) (SimulationResult, error)
}

// calculator é a implementação concreta de Engine.
type calculator struct{}

// NewCalculator retorna a implementação padrão do motor de cálculo.
func NewCalculator() Engine {
	return &calculator{}
}

// Calculate executa a simulação comparativa entre regime atual e projetado.
//
// Regime atual: PIS + COFINS (não-cumulativo) + ISS, aplicados sobre receita.
// Regime projetado: CBS + IBS sobre receita, com crédito integral sobre despesas elegíveis.
//
// Premissa: ISS não gera crédito no regime atual (cumulativo para serviços simples).
// CBS e IBS admitem crédito pleno sobre insumos elegíveis (não-cumulatividade ampla).
func (c *calculator) Calculate(_ context.Context, input SimulationInput) (SimulationResult, error) {
	if len(input.Services) == 0 {
		return SimulationResult{}, fmt.Errorf("calculator: nenhum servico informado")
	}

	rules := RulesForYear(input.Year)

	// --- Receita total ---
	totalRevenue := decimal.Zero
	for _, svc := range input.Services {
		if svc.Amount.IsNegative() {
			return SimulationResult{}, fmt.Errorf("calculator: servico %q com valor negativo", svc.ID)
		}
		totalRevenue = totalRevenue.Add(svc.Amount)
	}

	// --- Cenário atual: PIS + COFINS + ISS ---
	// PIS e COFINS incidem sobre o total da receita (já com fator de redução no ano).
	// ISS incide sobre cada serviço individualmente com sua alíquota própria.
	currentGross := totalRevenue.Mul(rules.CombinedCurrentRate())
	for _, svc := range input.Services {
		currentGross = currentGross.Add(svc.Amount.Mul(svc.ISSRate))
	}
	currentGross = currentGross.Round(2)

	// No regime atual (PIS/COFINS não-cumulativo), créditos existem sobre insumos,
	// mas o ISS é cumulativo e não gera crédito. Para simplificar a simulação,
	// aplicamos créditos de PIS/COFINS sobre despesas elegíveis no regime atual.
	currentCredits := decimal.Zero
	for _, exp := range input.Expenses {
		if exp.IsEligible {
			if exp.Amount.IsNegative() {
				return SimulationResult{}, fmt.Errorf("calculator: despesa %q com valor negativo", exp.ID)
			}
			currentCredits = currentCredits.Add(exp.Amount.Mul(rules.CombinedCurrentRate()))
		}
	}
	currentCredits = currentCredits.Round(2)
	currentNet := currentGross.Sub(currentCredits).Round(2)

	// --- Cenário projetado: CBS + IBS ---
	projectedGross := totalRevenue.Mul(rules.CombinedProjectedRate()).Round(2)

	projectedCredits := decimal.Zero
	for _, exp := range input.Expenses {
		if exp.IsEligible {
			projectedCredits = projectedCredits.Add(exp.Amount.Mul(rules.CombinedProjectedRate()))
		}
	}
	projectedCredits = projectedCredits.Round(2)
	projectedNet := projectedGross.Sub(projectedCredits).Round(2)

	// --- Delta ---
	delta := currentNet.Sub(projectedNet).Round(2)
	deltaPct := decimal.Zero
	if currentNet.IsPositive() {
		deltaPct = delta.Div(currentNet).Mul(decimal.NewFromInt(100)).Round(2)
	}

	return SimulationResult{
		Year: input.Year,
		Current: TaxBreakdown{
			GrossTax: currentGross,
			Credits:  currentCredits,
			NetTax:   currentNet,
		},
		Projected: TaxBreakdown{
			GrossTax: projectedGross,
			Credits:  projectedCredits,
			NetTax:   projectedNet,
		},
		Delta:    delta,
		DeltaPct: deltaPct,
	}, nil
}
