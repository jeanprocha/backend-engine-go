package tax

import (
	"strings"

	"github.com/shopspring/decimal"
)

// CreditLeakComputed representa uma despesa inelegível e o crédito hipotético não apropriado
// (mesma lógica de alíquota que computeProjectedCBSIBS para elegíveis).
type CreditLeakComputed struct {
	Description string
	Value       decimal.Decimal
	LostCredit  decimal.Decimal
	RegimeType  string // normalizado (para contexto da LLM)
	// LegalBase é a citação do RAG (Expense.LegalBase) — passthrough, nunca
	// inventada aqui; vazia se a classificação não tinha citação.
	LegalBase string
	// AnnualValues projeta LostCredit para cada ano 2026-2033, com a mesma
	// alíquota crescente que TransitionSeries usa para o resto da simulação
	// (rules.go) — não é um número novo, é o mesmo motor aplicado ano a ano.
	// Assume Value constante ao longo da transição: a mesma simplificação
	// que TransitionSeries já faz para toda a simulação (reexecuta Calculate
	// com o mesmo payload variando só o ano) — declarado, não escondido.
	AnnualValues []CreditLeakAnnualValue
	// Effort, Risk e Priority são faixas determinísticas — nunca escritas
	// pela LLM (ver EnrichCreditLeaks). Ver creditLeakEffort/creditLeakRisk/
	// creditLeakPriority para a derivação e o porquê de cada faixa.
	Effort   string // "baixo" | "medio" | "alto"
	Risk     string // "baixo" | "medio" | "alto"
	Priority string // "alta" | "media" | "baixa"
}

// CreditLeakAnnualValue é o crédito não apropriado projetado para um ano
// específico da transição — mesma despesa, alíquota efetiva daquele ano.
type CreditLeakAnnualValue struct {
	Year       int
	LostCredit decimal.Decimal
}

// Faixas de valor acumulado (R$, somado 2026-2033) para Risk/Priority — são
// heurística TribIA declarada, não figura legal (mesmo vocabulário de
// "premissa TribIA" usado em rules.go para prof_liberal/MEI/Simples).
var (
	creditLeakLowValueThreshold  = decimal.RequireFromString("3000")
	creditLeakHighValueThreshold = decimal.RequireFromString("15000")
)

// creditLeakEffort deriva o esforço de correção do regime_type: padrao e
// diferenciado_60 são reduções previstas na LC 214/2025 (ver
// EffectiveProjectedRate) — só exigem reclassificar a despesa. Já
// RegimeProfissionalLiberal usa uma redução ILUSTRATIVA da TribIA (30%, não
// prevista em lei), então exige validação humana antes de agir: esforço
// alto não por complexidade de cálculo, mas porque a premissa em si precisa
// ser confirmada primeiro.
func creditLeakEffort(regimeType string) string {
	switch regimeType {
	case RegimeProfissionalLiberal:
		return "alto"
	case RegimeDiferenciado60:
		return "medio"
	default: // RegimePadrao e qualquer valor não reconhecido normalizado para ele
		return "baixo"
	}
}

// creditLeakRisk deriva o risco de a correção ser contestada: valores
// maiores atraem mais escrutínio de auditoria; um regime de premissa TribIA
// (prof_liberal) é sempre "alto" independente do valor, pelo mesmo motivo
// do esforço alto acima.
func creditLeakRisk(regimeType string, totalAnnualized decimal.Decimal) string {
	if regimeType == RegimeProfissionalLiberal {
		return "alto"
	}
	switch {
	case totalAnnualized.LessThan(creditLeakLowValueThreshold):
		return "baixo"
	case totalAnnualized.GreaterThanOrEqual(creditLeakHighValueThreshold):
		return "alto"
	default:
		return "medio"
	}
}

// creditLeakPriority deriva a prioridade só do valor acumulado — critério
// explícito da PR5 ("prioridade derivada do valor"), sem o modificador de
// regime que Risk tem. É o que ordena a tabela do "Plano de ação" (PR6).
func creditLeakPriority(totalAnnualized decimal.Decimal) string {
	switch {
	case totalAnnualized.LessThan(creditLeakLowValueThreshold):
		return "baixa"
	case totalAnnualized.GreaterThanOrEqual(creditLeakHighValueThreshold):
		return "alta"
	default:
		return "media"
	}
}

// buildCreditLeakAnnualValues projeta lost credit para cada ano 2026-2033
// (transitionSeriesMinYear/MaxYear) — mesma Amount, alíquota efetiva de
// cada ano.
func buildCreditLeakAnnualValues(amount decimal.Decimal, regimeType string) []CreditLeakAnnualValue {
	out := make([]CreditLeakAnnualValue, 0, transitionSeriesMaxYear-transitionSeriesMinYear+1)
	for y := transitionSeriesMinYear; y <= transitionSeriesMaxYear; y++ {
		rate := RulesForYear(y).EffectiveProjectedRate(regimeType)
		out = append(out, CreditLeakAnnualValue{Year: y, LostCredit: amount.Mul(rate).Round(2)})
	}
	return out
}

func sumAnnualValues(values []CreditLeakAnnualValue) decimal.Decimal {
	total := decimal.Zero
	for _, v := range values {
		total = total.Add(v.LostCredit)
	}
	return total
}

// CreditLeaksSupported indica se o perfil da simulação apropria créditos projetados de forma
// que "vazamento" faça sentido (exclui MEI, Simples puro, entidade imune).
func CreditLeaksSupported(companyRegime string) bool {
	r := strings.TrimSpace(companyRegime)
	if strings.EqualFold(r, CompanyRegimeMEI) {
		return false
	}
	if IsEntidadeImuneProfile(r) {
		return false
	}
	if strings.EqualFold(r, CompanyRegimeSimplesPuro) {
		return false
	}
	return true
}

// NormalizeExpenseRegimeType alinha regime_type da despesa ao classificador/motor:
// vazio ou desconhecido vira RegimePadrao; preserva diferenciado_60 e reduzido_zero.
func NormalizeExpenseRegimeType(regime string) string {
	r := strings.TrimSpace(strings.ToLower(regime))
	switch r {
	case "", RegimePadrao, "regular":
		return RegimePadrao
	case RegimeDiferenciado60:
		return RegimeDiferenciado60
	case RegimeReduzidoZero:
		return RegimeReduzidoZero
	case RegimeProfissionalLiberal:
		return RegimeProfissionalLiberal
	default:
		return RegimePadrao
	}
}

// BuildCreditLeaks monta a lista de vazamentos para despesas com IsEligible == false.
// Ignora itens com lost_credit arredondado a zero (ex.: regime reduzido_zero com alíquota zero).
func BuildCreditLeaks(year int, expenses []Expense) []CreditLeakComputed {
	if len(expenses) == 0 {
		return nil
	}
	rules := RulesForYear(year)
	var out []CreditLeakComputed
	for _, e := range expenses {
		if e.IsEligible {
			continue
		}
		rt := NormalizeExpenseRegimeType(e.RegimeType)
		rate := rules.EffectiveProjectedRate(rt)
		lost := e.Amount.Mul(rate).Round(2)
		if lost.IsZero() {
			continue
		}
		annual := buildCreditLeakAnnualValues(e.Amount, rt)
		total := sumAnnualValues(annual)
		out = append(out, CreditLeakComputed{
			Description:  e.Description,
			Value:        e.Amount,
			LostCredit:   lost,
			RegimeType:   rt,
			LegalBase:    strings.TrimSpace(e.LegalBase),
			AnnualValues: annual,
			Effort:       creditLeakEffort(rt),
			Risk:         creditLeakRisk(rt, total),
			Priority:     creditLeakPriority(total),
		})
	}
	return out
}
