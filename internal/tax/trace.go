package tax

import "github.com/shopspring/decimal"

// CalculationStepInput é um operando nomeado de um CalculationStep — um valor
// que entrou em Formula. Sempre decimal.Decimal, nunca float64, mesma regra
// de todo o domínio fiscal.
type CalculationStepInput struct {
	Name  string
	Value decimal.Decimal
}

// CalculationStep é uma operação aritmética auditável dentro de um
// TaxBreakdown — precisa o suficiente para que alguém com só o dossiê em mãos
// reproduza Output a partir de Inputs via Formula, sem acesso ao código.
//
// W2/PR1 (docs/roadmap-execucao.md, Etapa C): o motor já computava cada um
// destes números internamente e os descartava antes desta PR — ver a tabela
// de achados no roadmap para exatamente quais linhas. Este tipo existe para
// parar de descartar, não para computar algo novo.
type CalculationStep struct {
	// Item identifica a linha a que este passo se refere (descrição/ID de um
	// serviço ou despesa) — vazio quando o passo não é sobre uma linha
	// específica (um agregado, uma alíquota, um subtotal).
	Item string
	// Label nomeia o que este passo calcula, em linguagem simples.
	Label string
	// Formula descreve a operação em termos humanos — quem lê combina isto
	// com Inputs para reproduzir Output à mão.
	Formula string
	Inputs  []CalculationStepInput
	Output  decimal.Decimal
	// Rounded é true quando Output foi arredondado (Round(2), tipicamente)
	// como parte deste passo — distingue um intermediário exato de um
	// arredondado. Um trace que soma seus próprios passos arredondados pode
	// divergir de um agregado que arredonda em outro ponto; isso não é erro
	// — é o mesmo fenômeno que TaxComponents já documenta.
	Rounded bool
}

// calcTrace acumula CalculationStep enquanto uma função de cálculo roda. Nada
// aqui muda o que é computado — só o que é registrado sobre como.
type calcTrace struct {
	steps []CalculationStep
}

func (t *calcTrace) add(item, label, formula string, output decimal.Decimal, rounded bool, inputs ...CalculationStepInput) {
	t.steps = append(t.steps, CalculationStep{
		Item:    item,
		Label:   label,
		Formula: formula,
		Inputs:  inputs,
		Output:  output,
		Rounded: rounded,
	})
}

// in é um construtor terso de CalculationStepInput, usado em cada chamada de calcTrace.add.
func in(name string, value decimal.Decimal) CalculationStepInput {
	return CalculationStepInput{Name: name, Value: value}
}

// lineItem nomeia um serviço/despesa para CalculationStep.Item: a descrição
// quando presente (o que quem lê reconhece), o ID caso contrário — nenhum dos
// dois campos é obrigatório na API.
func lineItem(id, description string) string {
	if description != "" {
		return description
	}
	if id != "" {
		return id
	}
	return "(sem descrição)"
}

// regimeDisplayLabel nomeia o regime tributário de um item para o rótulo do
// trace. "" e valores desconhecidos caem no mesmo tratamento de
// EffectiveProjectedRate (alíquota plena), por isso aparecem como "padrão".
func regimeDisplayLabel(regime string) string {
	switch regime {
	case RegimeDiferenciado60:
		return "diferenciado_60 (60% de redução)"
	case RegimeProfissionalLiberal:
		return "prof_liberal (30% de redução ilustrativa)"
	case RegimeReduzidoZero:
		return "reduzido_zero (alíquota zero)"
	default:
		return "padrão (alíquota plena)"
	}
}
