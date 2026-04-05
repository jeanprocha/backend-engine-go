package tax

import (
	"os"
	"strings"

	"github.com/shopspring/decimal"
)

// Constantes de regime tributario conforme a LC 68/2024.
// Usadas no campo RegimeType de Service e nos handlers HTTP.
const (
	// RegimePadrao: aliquota cheia de CBS+IBS (estimada ~26,5% plena em 2033).
	RegimePadrao = "padrao"
	// RegimeDiferenciado60: reducao de 60% na aliquota ? paga 40% da aliquota padrao.
	// Art. 131 LC 68/2024: Saude, Educacao, Dispositivos Medicos, Higiene Pessoal, etc.
	RegimeDiferenciado60 = "diferenciado_60"
	// RegimeProfissionalLiberal: reducao ilustrativa de 30% na aliquota; paga 70% da aliquota padrao (TribIA).
	// company_regime prof_liberal; profissoes regulamentadas. Nao substitui assessoria.
	RegimeProfissionalLiberal = "prof_liberal"
	// RegimeReduzidoZero: aliquota zero ? sem tributacao CBS/IBS.
	// Cesta Basica Nacional (Anexo I LC 68/2024) e demais hipoteses de isencao.
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
	// 1.0 = aliquota plena; 0.0 = extintos.
	// Referencia: Art. 345 ss. LC 68/2024 (transicao 2026-2033).
	PISCOFINSFactor decimal.Decimal

	// Regime projetado (IBS + CBS)
	// Art. 345 ? CBS cobrada a 0,9% a partir de 2026.
	CBSRate decimal.Decimal
	IBSRate decimal.Decimal
}

var (
	pisFull    = decimal.NewFromFloat(0.0165) // 1,65%
	cofinsFull = decimal.NewFromFloat(0.0760) // 7,60%
)

// RulesForYear retorna as aliquotas e fatores de reducao para o ano solicitado.
// Anos fora do intervalo 2026-2033 retornam as regras do ano mais proximo do intervalo.
//
// Premissas de transicao (LC 68/2024 ? aliquotas estimadas):
//   - 2026: CBS 0,9% + IBS 0,1% = 1,0% (fase de teste, Art. 345).
//   - 2027: CBS 1,5% + IBS 3,5% = 5,0%; PIS/COFINS reduzidos a 70%.
//   - 2028: CBS 3,0% + IBS 8,0% = 11,0%; PIS/COFINS reduzidos a 40%.
//   - 2029-2032: extincao gradual de PIS/COFINS; CBS+IBS crescem para 16,5-25,0%.
//   - 2033+: PIS/COFINS extintos, CBS 9,9% + IBS 16,6% = 26,5% plenos.
//
// CBS e IBS crescem em conjunto a medida que PIS/COFINS sao extintos.
// Aliquota IBS de referencia (16,6%) e estimada ? a lei delega fixacao a lei complementar
// dos estados/municipios (Art. 156-A CF/88).
func RulesForYear(year int) TaxRules {
	if year < 2026 {
		year = 2026
	}
	if year > 2033 {
		year = 2033
	}

	type yearConfig struct {
		pisCofins float64 // fator de manutencao (1.0 = pleno)
		cbs       float64 // aliquota CBS crescente
		ibs       float64 // aliquota IBS crescente
	}

	// CBS + IBS por ano de transicao (estimativas baseadas na LC 68/2024).
	// Soma CBS+IBS atinge 26,5% em 2033 (aliquota de referencia plena).
	configs := map[int]yearConfig{
		2026: {1.000, 0.009, 0.001}, // total 1,0% ? fase de teste
		2027: {0.700, 0.015, 0.035}, // total 5,0%
		2028: {0.400, 0.030, 0.080}, // total 11,0%
		2029: {0.225, 0.050, 0.115}, // total 16,5%
		2030: {0.150, 0.065, 0.135}, // total 20,0%
		2031: {0.075, 0.080, 0.150}, // total 23,0%
		2032: {0.000, 0.090, 0.160}, // total 25,0%
		2033: {0.000, 0.099, 0.166}, // total 26,5% ? aliquota plena de referencia
	}

	cfg := configs[year]
	factor := decimal.NewFromFloat(cfg.pisCofins)
	cbsRate := decimal.NewFromFloat(cfg.cbs)
	ibsRate := decimal.NewFromFloat(cfg.ibs)

	return TaxRules{
		Year:            year,
		PISRate:         pisFull.Mul(factor).Round(6),
		COFINSRate:      cofinsFull.Mul(factor).Round(6),
		PISCOFINSFactor: factor,
		CBSRate:         cbsRate,
		IBSRate:         ibsRate,
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

// EffectiveProjectedRate retorna a aliquota CBS+IBS efetiva dado o regime tributario.
//
// Regimes diferenciados (Art. 131 LC 68/2024):
//   - RegimeDiferenciado60: 60% de reducao ? paga 40% da aliquota padrao.
//   - RegimeProfissionalLiberal: reducao ilustrativa de 30% ? paga 70% da aliquota padrao.
//   - RegimeReduzidoZero: aliquota zero (cesta basica e isencoes do Anexo I).
//   - RegimePadrao (ou valor vazio/desconhecido): aliquota plena.
func (r TaxRules) EffectiveProjectedRate(regime string) decimal.Decimal {
	base := r.CombinedProjectedRate()
	switch regime {
	case RegimeDiferenciado60:
		return base.Mul(decimal.NewFromFloat(0.4)).Round(6)
	case RegimeProfissionalLiberal:
		return base.Mul(decimal.NewFromFloat(0.7)).Round(6)
	case RegimeReduzidoZero:
		return decimal.Zero
	default:
		return base
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
