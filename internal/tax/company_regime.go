package tax

import (
	"os"
	"strings"

	"github.com/shopspring/decimal"
)

// Valores de company_regime no JSON (perfil da empresa).
// Não confundir com RegimePadrao / regime_type dos itens (LC 68/2024).
const (
	CompanyRegimeRegular        = "regular"
	CompanyRegimeSimplesPuro    = "simples_puro"
	CompanyRegimeSimplesHibrido = "simples_hibrido"
	// CompanyRegimeSectorDiferenciado60 é perfil da empresa (company_regime no JSON).
	// Mesmo valor que regime_type "diferenciado_60" na LC 68/2024: projeção de saída força redução de 60%.
	CompanyRegimeSectorDiferenciado60 = "diferenciado_60"
	// CompanyRegimeAliquotaZero é perfil da empresa (cesta básica / social, ilustrativo).
	// Distinto de regime_type "reduzido_zero" na LC 68/2024: projeção força alíquota CBS+IBS zero em toda a receita.
	CompanyRegimeAliquotaZero = "aliquota_zero"
	// CompanyRegimeImobiliarioVenda: incorporação / venda (ilustrativo) — projeção CBS+IBS = 60% da alíquota padrão do ano.
	CompanyRegimeImobiliarioVenda = "imobiliario_venda"
	// CompanyRegimeImobiliarioAluguel: locação / arrendamento (ilustrativo) — projeção CBS+IBS = 40% da alíquota padrão do ano.
	CompanyRegimeImobiliarioAluguel = "imobiliario_aluguel"
)

// SimplesIllustrativeCurrentRate retorna taxa ilustrativa sobre faturamento para o
// "regime atual" nos perfis Simples Nacional (baseline compartilhado puro vs híbrido).
// Override: SIMPLES_ILLUSTRATIVE_CURRENT_RATE (fração decimal, ex. "0.06"). Ilustrativo: não substitui assessoria.
func SimplesIllustrativeCurrentRate() decimal.Decimal {
	return parseFractionEnv("SIMPLES_ILLUSTRATIVE_CURRENT_RATE", "0.06")
}

// SimplesPuroEffectiveIBSCBSRate modela IBS/CBS embutidos no DAS (Simples puro — crédito restrito).
// Override: SIMPLES_PURO_EFFECTIVE_IBS_CBS (ex. "0.04").
func SimplesPuroEffectiveIBSCBSRate() decimal.Decimal {
	return parseFractionEnv("SIMPLES_PURO_EFFECTIVE_IBS_CBS", "0.04")
}

func parseFractionEnv(key, defaultVal string) decimal.Decimal {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		s = defaultVal
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(1)) {
		d, _ = decimal.NewFromString(defaultVal)
	}
	return d
}

func parseMoneyEnv(key, defaultVal string) decimal.Decimal {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		s = defaultVal
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() {
		d, _ = decimal.NewFromString(defaultVal)
	}
	return d
}

// ImobiliarioRedutorDefaultBRL retorna redutor de ajuste ilustrativo (base em R$) quando o JSON não envia valor.
// IMOBILIARIO_REDUTOR_VENDA_BRL / IMOBILIARIO_REDUTOR_ALUGUEL_BRL (ex.: "40000.00"). Não substitui assessoria.
func ImobiliarioRedutorDefaultBRL(companyRegime string) decimal.Decimal {
	switch {
	case IsImobiliarioVendaProfile(companyRegime):
		return parseMoneyEnv("IMOBILIARIO_REDUTOR_VENDA_BRL", "0")
	case IsImobiliarioAluguelProfile(companyRegime):
		return parseMoneyEnv("IMOBILIARIO_REDUTOR_ALUGUEL_BRL", "0")
	default:
		return decimal.Zero
	}
}

// IsSimplesNationalProfile indica se o perfil usa baseline Simples ilustrativo no atual.
func IsSimplesNationalProfile(companyRegime string) bool {
	r := strings.ToLower(strings.TrimSpace(companyRegime))
	return r == CompanyRegimeSimplesPuro || r == CompanyRegimeSimplesHibrido
}

// IsSectorDiferenciado60Profile indica perfil setorial (saúde, educação, cultura etc.): atual = regular; projetado com alíquota reduzida em toda a receita.
func IsSectorDiferenciado60Profile(companyRegime string) bool {
	return strings.EqualFold(strings.TrimSpace(companyRegime), CompanyRegimeSectorDiferenciado60)
}

// IsAliquotaZeroProfile indica perfil cesta básica / social (ilustrativo): atual = regular; projetado CBS+IBS zero na saída.
func IsAliquotaZeroProfile(companyRegime string) bool {
	return strings.EqualFold(strings.TrimSpace(companyRegime), CompanyRegimeAliquotaZero)
}

// IsImobiliarioVendaProfile indica perfil incorporação / venda (projeção com 60% da alíquota padrão CBS+IBS do ano).
func IsImobiliarioVendaProfile(companyRegime string) bool {
	return strings.EqualFold(strings.TrimSpace(companyRegime), CompanyRegimeImobiliarioVenda)
}

// IsImobiliarioAluguelProfile indica perfil locação (projeção com 40% da alíquota padrão CBS+IBS do ano).
func IsImobiliarioAluguelProfile(companyRegime string) bool {
	return strings.EqualFold(strings.TrimSpace(companyRegime), CompanyRegimeImobiliarioAluguel)
}

// IsImobiliarioProfile agrupa os dois perfis imobiliários do TribIA.
func IsImobiliarioProfile(companyRegime string) bool {
	return IsImobiliarioVendaProfile(companyRegime) || IsImobiliarioAluguelProfile(companyRegime)
}
