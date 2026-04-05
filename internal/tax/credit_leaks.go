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
		out = append(out, CreditLeakComputed{
			Description: e.Description,
			Value:       e.Amount,
			LostCredit:  lost,
			RegimeType:  rt,
		})
	}
	return out
}
