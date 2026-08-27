package tax

import (
	"strings"

	"github.com/shopspring/decimal"
)

// Valores de company_regime no JSON (perfil da empresa).
// Não confundir com RegimePadrao / regime_type dos itens (LC 214/2025).
const (
	CompanyRegimeRegular        = "regular"
	CompanyRegimeSimplesPuro    = "simples_puro"
	CompanyRegimeSimplesHibrido = "simples_hibrido"
	// CompanyRegimeSectorDiferenciado60 é perfil da empresa (company_regime no JSON).
	// Mesmo valor que regime_type "diferenciado_60" na LC 214/2025: projeção de saída força redução de 60%.
	CompanyRegimeSectorDiferenciado60 = "diferenciado_60"
	// CompanyRegimeAliquotaZero é perfil da empresa (cesta básica / social, ilustrativo).
	// Distinto de regime_type "reduzido_zero" na LC 214/2025: projeção força alíquota CBS+IBS zero em toda a receita.
	CompanyRegimeAliquotaZero = "aliquota_zero"
	// CompanyRegimeImobiliarioVenda: incorporação / venda (ilustrativo) — projeção CBS+IBS = 60% da alíquota padrão do ano.
	CompanyRegimeImobiliarioVenda = "imobiliario_venda"
	// CompanyRegimeImobiliarioAluguel: locação / arrendamento (ilustrativo) — projeção CBS+IBS = 40% da alíquota padrão do ano.
	CompanyRegimeImobiliarioAluguel = "imobiliario_aluguel"
	// CompanyRegimeProfissionalLiberal: profissoes regulamentadas (ilustrativo) — projeção CBS+IBS = 70% da alíquota padrão do ano.
	CompanyRegimeProfissionalLiberal = "prof_liberal"
	// CompanyRegimeExportadora: exportação (ilustrativo, Art. 52 LC 214/2025 — TODO W1-onda2: confirmar numeração) — projeção CBS+IBS zero na saída; créditos nas entradas.
	CompanyRegimeExportadora = "exportadora"
	// CompanyRegimeEntidadeImune: entidades imunes / ISFL (ilustrativo) — projeção CBS+IBS zero na saída; sem apropriação de créditos no modelo.
	CompanyRegimeEntidadeImune = "entidade_imune"
)

// IsKnownCompanyRegime indica se companyRegime é reconhecido pelo motor —
// vazio conta como conhecido (mesmo comportamento de "regular"). Usado por
// Calculate (W7/B2.2) para rejeitar company_regime desconhecido em vez de
// cair silenciosamente no ramo "regular".
func IsKnownCompanyRegime(companyRegime string) bool {
	r := strings.TrimSpace(companyRegime)
	if r == "" {
		return true
	}
	switch strings.ToLower(r) {
	case CompanyRegimeRegular, CompanyRegimeMEI, CompanyRegimeSimplesPuro, CompanyRegimeSimplesHibrido,
		CompanyRegimeSectorDiferenciado60, CompanyRegimeAliquotaZero, CompanyRegimeImobiliarioVenda,
		CompanyRegimeImobiliarioAluguel, CompanyRegimeProfissionalLiberal, CompanyRegimeExportadora,
		CompanyRegimeEntidadeImune:
		return true
	default:
		return false
	}
}

// Parâmetros fiscais ilustrativos, congelados como constante de pacote
// (W7/B2.2 — docs/roadmap-execucao.md). Antes eram lidos de variável de
// ambiente a cada chamada de Calculate: o mesmo input podia devolver
// resultado diferente entre deploys, contradizendo "cada número é
// reproduzível" (PRODUCT.md) e inviabilizando a suíte cruzada contra a
// Calculadora RFB (B2.1) — um golden gerado numa máquina não seria
// reproduzível noutra. Nenhuma das cinco estava documentada em
// .env.example; os valores abaixo são exatamente os defaults já em uso
// em produção — zero mudança de comportamento além de tornar os overrides
// (nunca configurados) inoperantes. Se uma premissa precisar ser ajustável
// no futuro, o caminho é entrada da API (como ImobiliarioRedutorAjusteBRL
// já é em SimulationInput), não variável de ambiente.
const (
	// simplesIllustrativeCurrentRate: taxa ilustrativa sobre faturamento para o
	// "regime atual" nos perfis Simples Nacional (baseline puro vs híbrido).
	simplesIllustrativeCurrentRateStr = "0.06"
	// simplesPuroEffectiveIBSCBSRate: IBS/CBS embutidos no DAS (Simples puro — crédito restrito).
	simplesPuroEffectiveIBSCBSRateStr = "0.04"
)

// SimplesIllustrativeCurrentRate retorna a taxa ilustrativa sobre faturamento
// para o "regime atual" nos perfis Simples Nacional. Ilustrativo: não
// substitui assessoria.
func SimplesIllustrativeCurrentRate() decimal.Decimal {
	return decimal.RequireFromString(simplesIllustrativeCurrentRateStr)
}

// SimplesPuroEffectiveIBSCBSRate modela IBS/CBS embutidos no DAS (Simples puro — crédito restrito).
func SimplesPuroEffectiveIBSCBSRate() decimal.Decimal {
	return decimal.RequireFromString(simplesPuroEffectiveIBSCBSRateStr)
}

// ImobiliarioRedutorDefaultBRL retorna redutor de ajuste ilustrativo (base em
// R$) quando o JSON não envia valor — hoje sempre zero para os dois perfis
// imobiliários (venda/aluguel; companyRegime mantido na assinatura para não
// quebrar o único call site, resolveImobiliarioRedutor, que já garante o
// perfil antes de chamar). Não substitui assessoria.
func ImobiliarioRedutorDefaultBRL(companyRegime string) decimal.Decimal {
	_ = companyRegime
	return decimal.Zero
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

// IsExportadoraProfile indica perfil exportação (ilustrativo): atual = regular; projetado CBS+IBS zero na saída (imunidade na receita);
// créditos por despesa — mesmo trilho numérico que aliquota_zero, slug distinto para UX e RAG.
func IsExportadoraProfile(companyRegime string) bool {
	return strings.EqualFold(strings.TrimSpace(companyRegime), CompanyRegimeExportadora)
}

// IsEntidadeImuneProfile indica perfil entidade imune / consumidor final ilustrativo: atual = regular (baseline);
// projetado CBS+IBS zero na saída e créditos projetados forçados a zero (sem apropriação no modelo).
func IsEntidadeImuneProfile(companyRegime string) bool {
	return strings.EqualFold(strings.TrimSpace(companyRegime), CompanyRegimeEntidadeImune)
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

// IsProfissionalLiberalProfile indica perfil profissões regulamentadas (ilustrativo): atual = regular; projetado = 70% da alíquota CBS+IBS padrão do ano.
func IsProfissionalLiberalProfile(companyRegime string) bool {
	return strings.EqualFold(strings.TrimSpace(companyRegime), CompanyRegimeProfissionalLiberal)
}
