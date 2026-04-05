package tax

import "github.com/shopspring/decimal"

// Service representa uma receita de saída da empresa (serviço prestado).
type Service struct {
	ID          string
	Description string
	Amount      decimal.Decimal // valor bruto do serviço
	ISSRate     decimal.Decimal // alíquota ISS vigente (ex: 0.05 = 5%)
}

// Expense representa uma despesa que pode gerar crédito tributário.
// IsEligible é definido pelo usuário ou classificado pelo RAG.
type Expense struct {
	ID          string
	Description string
	Amount      decimal.Decimal
	IsEligible  bool
}

// SimulationInput reúne todos os dados necessários para uma simulação.
// Year deve estar entre 2026 e 2033 (período de transição da LC 68/2024).
type SimulationInput struct {
	Year     int
	Services []Service
	Expenses []Expense
}

// TaxBreakdown detalha os componentes de um cenário tributário.
type TaxBreakdown struct {
	GrossTax decimal.Decimal // imposto sobre a saída (receita)
	Credits  decimal.Decimal // créditos sobre a entrada elegível
	NetTax   decimal.Decimal // imposto líquido = GrossTax - Credits
}

// SimulationResult compara o regime atual (PIS/COFINS/ISS) com o projetado (IBS/CBS).
// Delta positivo significa economia com o novo regime.
type SimulationResult struct {
	Year      int
	Current   TaxBreakdown // regime atual
	Projected TaxBreakdown // regime projetado (IBS + CBS)
	Delta     decimal.Decimal // Current.NetTax - Projected.NetTax
	DeltaPct  decimal.Decimal // Delta / Current.NetTax * 100 (quando Current > 0)
}
