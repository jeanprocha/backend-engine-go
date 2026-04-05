package tax

import "github.com/shopspring/decimal"

// Constantes de regime tributário conforme a LC 68/2024.
// Usadas no campo RegimeType de Service e nos handlers HTTP.
const (
	// RegimePadrao: alíquota cheia de CBS+IBS (estimada ~26,5% plena em 2033).
	RegimePadrao = "padrao"
	// RegimeDiferenciado60: redução de 60% na alíquota — paga 40% da alíquota padrão.
	// Art. 131 LC 68/2024: Saúde, Educação, Dispositivos Médicos, Higiene Pessoal, etc.
	RegimeDiferenciado60 = "diferenciado_60"
	// RegimeReduzidoZero: alíquota zero — sem tributação CBS/IBS.
	// Cesta Básica Nacional (Anexo I LC 68/2024) e demais hipóteses de isenção.
	RegimeReduzidoZero = "reduzido_zero"
)

// TaxRules contém as alíquotas aplicáveis em um dado ano de transição.
// Todas as alíquotas são decimais fracionários (ex: 0.009 = 0,9%).
type TaxRules struct {
	Year int

	// Regime atual (PIS/COFINS/ISS)
	// PIS e COFINS são reduzidos progressivamente durante a transição.
	PISRate    decimal.Decimal // base: 1,65%
	COFINSRate decimal.Decimal // base: 7,60%

	// PISCOFINSFactor é o fator de redução proporcional aplicado ao regime atual.
	// 1.0 = alíquota plena; 0.0 = extintos.
	// Referência: Art. 345 ss. LC 68/2024 (transição 2026–2033).
	PISCOFINSFactor decimal.Decimal

	// Regime projetado (IBS + CBS)
	// Art. 345 – CBS cobrada a 0,9% a partir de 2026.
	CBSRate decimal.Decimal
	IBSRate decimal.Decimal
}

var (
	pisFull    = decimal.NewFromFloat(0.0165) // 1,65%
	cofinsFull = decimal.NewFromFloat(0.0760) // 7,60%
	one        = decimal.NewFromInt(1)
)

// RulesForYear retorna as alíquotas e fatores de redução para o ano solicitado.
// Anos fora do intervalo 2026–2033 retornam as regras do ano mais próximo do intervalo.
//
// Premissas de transição (LC 68/2024 — alíquotas estimadas):
//   - 2026: CBS 0,9% + IBS 0,1% = 1,0% (fase de teste, Art. 345).
//   - 2027: CBS 1,5% + IBS 3,5% = 5,0%; PIS/COFINS reduzidos a 70%.
//   - 2028: CBS 3,0% + IBS 8,0% = 11,0%; PIS/COFINS reduzidos a 40%.
//   - 2029–2032: extinção gradual de PIS/COFINS; CBS+IBS crescem para 16,5–25,0%.
//   - 2033+: PIS/COFINS extintos, CBS 9,9% + IBS 16,6% = 26,5% plenos.
//
// CBS e IBS crescem em conjunto à medida que PIS/COFINS são extintos.
// Alíquota IBS de referência (16,6%) é estimada — a lei delega fixação a lei complementar
// dos estados/municípios (Art. 156-A CF/88).
func RulesForYear(year int) TaxRules {
	if year < 2026 {
		year = 2026
	}
	if year > 2033 {
		year = 2033
	}

	type yearConfig struct {
		pisCofins float64 // fator de manutenção (1.0 = pleno)
		cbs       float64 // alíquota CBS crescente
		ibs       float64 // alíquota IBS crescente
	}

	// CBS + IBS por ano de transição (estimativas baseadas na LC 68/2024).
	// Soma CBS+IBS atinge 26,5% em 2033 (alíquota de referência plena).
	configs := map[int]yearConfig{
		2026: {1.000, 0.009, 0.001}, // total 1,0%  — fase de teste
		2027: {0.700, 0.015, 0.035}, // total 5,0%
		2028: {0.400, 0.030, 0.080}, // total 11,0%
		2029: {0.225, 0.050, 0.115}, // total 16,5%
		2030: {0.150, 0.065, 0.135}, // total 20,0%
		2031: {0.075, 0.080, 0.150}, // total 23,0%
		2032: {0.000, 0.090, 0.160}, // total 25,0%
		2033: {0.000, 0.099, 0.166}, // total 26,5% — alíquota plena de referência
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

// CombinedCurrentRate retorna PIS + COFINS somados (útil para créditos no regime atual).
func (r TaxRules) CombinedCurrentRate() decimal.Decimal {
	return r.PISRate.Add(r.COFINSRate)
}

// CombinedProjectedRate retorna CBS + IBS somados (alíquota padrão plena).
func (r TaxRules) CombinedProjectedRate() decimal.Decimal {
	return r.CBSRate.Add(r.IBSRate)
}

// EffectiveProjectedRate retorna a alíquota CBS+IBS efetiva dado o regime tributário.
//
// Regimes diferenciados (Art. 131 LC 68/2024):
//   - RegimeDiferenciado60: 60% de redução → paga 40% da alíquota padrão.
//   - RegimeReduzidoZero: alíquota zero (cesta básica e isenções do Anexo I).
//   - RegimePadrao (ou valor vazio/desconhecido): alíquota plena.
func (r TaxRules) EffectiveProjectedRate(regime string) decimal.Decimal {
	base := r.CombinedProjectedRate()
	switch regime {
	case RegimeDiferenciado60:
		return base.Mul(decimal.NewFromFloat(0.4)).Round(6)
	case RegimeReduzidoZero:
		return decimal.Zero
	default:
		return base
	}
}
