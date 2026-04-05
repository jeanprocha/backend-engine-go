package tax

import "github.com/shopspring/decimal"

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
	cbs2026    = decimal.NewFromFloat(0.009)  // 0,9% (Art. 345)
	ibs2026    = decimal.NewFromFloat(0.001)  // 0,1% (referência inicial)
	one        = decimal.NewFromInt(1)
)

// RulesForYear retorna as alíquotas e fatores de redução para o ano solicitado.
// Anos fora do intervalo 2026–2033 retornam as regras do ano mais próximo do intervalo.
//
// Premissas de transição (LC 68/2024):
//   - 2026: CBS entra a 0,9%; PIS/COFINS sem redução ainda.
//   - 2027: PIS/COFINS reduzidos a 70% (fator 0,70).
//   - 2028: reduzidos a 40% (fator 0,40).
//   - 2029–2032: extinção gradual (100%→0% em 4 anos iguais: 0,75 / 0,50 / 0,25 / 0,00).
//   - 2033+: PIS/COFINS extintos, IBS/CBS em alíquota plena de referência.
//
// IBS cresce simetricamente à redução do PIS/COFINS para preservar a carga total estimada.
func RulesForYear(year int) TaxRules {
	if year < 2026 {
		year = 2026
	}
	if year > 2033 {
		year = 2033
	}

	type yearConfig struct {
		pisCofins float64 // fator de manutenção (1.0 = pleno)
		ibs       float64 // alíquota IBS crescente
	}

	configs := map[int]yearConfig{
		2026: {1.00, 0.001},
		2027: {0.70, 0.031},
		2028: {0.40, 0.061},
		2029: {0.75 * 0.30, 0.076}, // 22,5% do original
		2030: {0.50 * 0.30, 0.091},
		2031: {0.25 * 0.30, 0.106},
		2032: {0.00, 0.121},
		2033: {0.00, 0.136}, // alíquota plena referência
	}

	cfg := configs[year]
	factor := decimal.NewFromFloat(cfg.pisCofins)
	ibsRate := decimal.NewFromFloat(cfg.ibs)

	return TaxRules{
		Year:            year,
		PISRate:         pisFull.Mul(factor).Round(6),
		COFINSRate:      cofinsFull.Mul(factor).Round(6),
		PISCOFINSFactor: factor,
		CBSRate:         cbs2026,
		IBSRate:         ibsRate,
	}
}

// CombinedCurrentRate retorna PIS + COFINS somados (útil para créditos no regime atual).
func (r TaxRules) CombinedCurrentRate() decimal.Decimal {
	return r.PISRate.Add(r.COFINSRate)
}

// CombinedProjectedRate retorna CBS + IBS somados.
func (r TaxRules) CombinedProjectedRate() decimal.Decimal {
	return r.CBSRate.Add(r.IBSRate)
}

// one é exportada para uso nos testes sem import desnecessário.
var One = one
