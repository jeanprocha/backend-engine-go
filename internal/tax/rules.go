package tax

import (
	"os"
	"strings"

	"github.com/shopspring/decimal"
)

// Constantes de regime tributario conforme a LC 214/2025.
// TODO(W1-onda2): confirmar regras/numeração contra o texto sancionado —
// estas constantes foram escritas com base no PLP 68/2024 (pré-sanção).
// Usadas no campo RegimeType de Service e nos handlers HTTP.
const (
	// RegimePadrao: aliquota cheia de CBS+IBS (estimada ~26,5% plena em 2033).
	RegimePadrao = "padrao"
	// RegimeDiferenciado60: reducao de 60% na aliquota ? paga 40% da aliquota padrao.
	// Art. 131 LC 214/2025 (TODO W1-onda2: confirmar numeração): Saude, Educacao, Dispositivos Medicos, Higiene Pessoal, etc.
	RegimeDiferenciado60 = "diferenciado_60"
	// RegimeProfissionalLiberal: reducao ilustrativa de 30% na aliquota; paga 70% da aliquota padrao (TribIA).
	// company_regime prof_liberal; profissoes regulamentadas. Nao substitui assessoria.
	RegimeProfissionalLiberal = "prof_liberal"
	// RegimeReduzidoZero: aliquota zero ? sem tributacao CBS/IBS.
	// Cesta Basica Nacional (Anexo I LC 214/2025, TODO W1-onda2: confirmar numeração) e demais hipoteses de isencao.
	RegimeReduzidoZero = "reduzido_zero"
)

// TaxRules contem as aliquotas aplicaveis em um dado ano de transicao.
// Todas as aliquotas sao decimais fracionarios (ex: 0.009 = 0,9%).
type TaxRules struct {
	Year int

	// Regime atual (PIS/COFINS/ISS)
	// PIS e COFINS sao reduzidos progressivamente durante a transicao.
	PISRate    decimal.Decimal // base: 1,65%
	COFINSRate decimal.Decimal // base: 7,60%

	// PISCOFINSFactor e o fator de reducao proporcional aplicado ao regime atual.
	// 1.0 = aliquota plena; 0.0 = extintos (a partir de 2027 — ver RulesForYear).
	// TODO(W1-onda2): confirmar numeração do dispositivo contra o texto sancionado.
	PISCOFINSFactor decimal.Decimal

	// Regime projetado (IBS + CBS) — ver RulesForYear para o calendário por ano.
	CBSRate decimal.Decimal
	IBSRate decimal.Decimal
}

var (
	pisFull    = decimal.RequireFromString("0.0165") // 1,65%
	cofinsFull = decimal.RequireFromString("0.0760") // 7,60%
)

// RulesForYear retorna as aliquotas e fatores de reducao para o ano solicitado.
// Anos fora do intervalo 2026-2033 retornam as regras do ano mais proximo do intervalo.
//
// Os valores vêm de transitionTable (transition_table.go) — uma linha por
// ano, cada uma com proveniência declarada em RuleBasis (W7/B2.2). Resumo do
// calendário efetivamente aplicado:
//   - 2026: CBS 0,9% + IBS 0,1% (fase-teste).
//   - 2027-2028: PIS/COFINS extintos; CBS ~8,7% (referência menos redução
//     compensatória de 0,1 p.p.); IBS nominal em 0,1%.
//   - 2029-2032: IBS sobe 10/20/30/40% da referência; ICMS/ISS caem na mesma
//     proporção (rampa de 1/10 ao ano, não 1/5 — corrigido nesta versão).
//   - 2033: vigência integral, CBS 8,8% + IBS 17,7% = 26,5% (alíquota de
//     referência, ainda projeção do MF/TCU — ver RuleBasis).
//
// Ver TransitionYearBasis(year) para a proveniência completa por ano.
func RulesForYear(year int) TaxRules {
	row := transitionRow(year)

	return TaxRules{
		Year:            row.Year,
		PISRate:         pisFull.Mul(row.PISCOFINSFactor).Round(6),
		COFINSRate:      cofinsFull.Mul(row.PISCOFINSFactor).Round(6),
		PISCOFINSFactor: row.PISCOFINSFactor,
		CBSRate:         row.CBSRate,
		IBSRate:         row.IBSRate,
	}
}

// CombinedCurrentRate retorna PIS + COFINS somados (util para creditos no regime atual).
func (r TaxRules) CombinedCurrentRate() decimal.Decimal {
	return r.PISRate.Add(r.COFINSRate)
}

// CombinedProjectedRate retorna CBS + IBS somados (aliquota padrao plena).
func (r TaxRules) CombinedProjectedRate() decimal.Decimal {
	return r.CBSRate.Add(r.IBSRate)
}

// ISSMunicipalTransitionFactor multiplica a alíquota de ISS informada no input no regime legado,
// modelando a extinção gradual do componente municipal na mesma proporção do ICMS
// (rampa de 1/10 ao ano, 2029-2032 — ver transitionTable/RuleBasis em transition_table.go).
// 2026-2028: 100%; 2029: 90%; 2030: 80%; 2031: 70%; 2032: 60%; 2033: ISS zero no legado.
func (r TaxRules) ISSMunicipalTransitionFactor() decimal.Decimal {
	return transitionRow(r.Year).ISSFactor
}

// EffectiveProjectedRate retorna a aliquota CBS+IBS efetiva dado o regime tributario.
//
// Regimes diferenciados (Art. 131 LC 214/2025, TODO W1-onda2: confirmar numeração):
//   - RegimeDiferenciado60: 60% de reducao ? paga 40% da aliquota padrao.
//   - RegimeProfissionalLiberal: reducao ilustrativa de 30% ? paga 70% da aliquota padrao.
//   - RegimeReduzidoZero: aliquota zero (cesta basica e isencoes do Anexo I).
//   - RegimePadrao (ou valor vazio/desconhecido): aliquota plena.
func (r TaxRules) EffectiveProjectedRate(regime string) decimal.Decimal {
	base := r.CombinedProjectedRate()
	switch regime {
	case RegimeDiferenciado60:
		return base.Mul(decimal.RequireFromString("0.4")).Round(6)
	case RegimeProfissionalLiberal:
		return base.Mul(decimal.RequireFromString("0.7")).Round(6)
	case RegimeReduzidoZero:
		return decimal.Zero
	default:
		return base
	}
}

// EffectiveProjectedRateSplit decompõe EffectiveProjectedRate em CBS e IBS
// separados, aplicando o mesmo multiplicador por regime a cada tributo —
// insumo só para popular TaxComponents (W7/B2.1), nunca para recalcular
// GrossTax (que continua vindo de EffectiveProjectedRate).
func (r TaxRules) EffectiveProjectedRateSplit(regime string) (cbs, ibs decimal.Decimal) {
	switch regime {
	case RegimeDiferenciado60:
		mult := decimal.RequireFromString("0.4")
		return r.CBSRate.Mul(mult).Round(6), r.IBSRate.Mul(mult).Round(6)
	case RegimeProfissionalLiberal:
		mult := decimal.RequireFromString("0.7")
		return r.CBSRate.Mul(mult).Round(6), r.IBSRate.Mul(mult).Round(6)
	case RegimeReduzidoZero:
		return decimal.Zero, decimal.Zero
	default:
		return r.CBSRate, r.IBSRate
	}
}

// CompanyRegimeMEI e o valor de company_regime no JSON para perfil MEI (DAS fixo mensal).
const CompanyRegimeMEI = "mei"

// MEIMonthlyDAS retem valor mensal ilustrativo do DAS para simulacao em base mensal.
// Override: variavel de ambiente MEI_MONTHLY_DAS_BRL (ex.: "85.50"). Nao substitui
// assessoria: nao modela anexo, funcionario nem teto de faturamento.
func MEIMonthlyDAS() decimal.Decimal {
	s := strings.TrimSpace(os.Getenv("MEI_MONTHLY_DAS_BRL"))
	if s == "" {
		return decimal.NewFromInt(85)
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() {
		return decimal.NewFromInt(85)
	}
	return d
}
