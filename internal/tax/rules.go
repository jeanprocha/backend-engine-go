package tax

import (
	"github.com/shopspring/decimal"
)

// Constantes de regime tributario conforme a LC 214/2025.
// Numeração auditada contra o texto compilado ingerido na Onda 2/W1
// (docs/lc214_2025_limpa.md) — ver o comentário de cada constante para a
// citação e para o que a auditoria corrigiu ou deixou em aberto.
// Usadas no campo RegimeType de Service e nos handlers HTTP.
const (
	// RegimePadrao: aliquota cheia de CBS+IBS (27,0% plena em 2033, validado
	// contra a Calculadora oficial da RFB em 28/08/2026 — W7/B2.1).
	RegimePadrao = "padrao"
	// RegimeDiferenciado60: reducao de 60% na aliquota — paga 40% da aliquota padrao.
	//
	// NÃO é um único artigo. É o Título IV, Capítulo II da LC 214/2025 —
	// instituído pelo Art. 126 (disposições gerais do regime), com o
	// percentual fixado ARTIGO A ARTIGO por categoria: Art. 129 (educação),
	// Art. 130 (saúde), Art. 131 (dispositivos médicos), Art. 132
	// (acessibilidade), Art. 133 (medicamentos registrados), Art. 135
	// (alimentos para consumo humano), Art. 136 (higiene pessoal), entre
	// outras — auditado, Onda 2/W1. "Art. 131" citava só a fatia de
	// dispositivos médicos como se fosse o regime inteiro.
	RegimeDiferenciado60 = "diferenciado_60"
	// RegimeProfissionalLiberal: reducao ilustrativa de 30% na aliquota; paga 70% da aliquota padrao (TribIA).
	// company_regime prof_liberal; profissoes regulamentadas. Nao substitui assessoria.
	//
	// Achado da auditoria (Onda 2/W1): existe uma redução de 30% REAL na lei —
	// Art. 127 —, mas para uma lista ENUMERADA de profissões regulamentadas
	// (administradores, advogados, arquitetos, contabilistas, economistas,
	// engenheiros, veterinários, entre outras), que NÃO inclui, por exemplo,
	// médicos e dentistas (cobertos por saúde, 60%, Art. 130). O modelo aplica
	// 30% a QUALQUER company_regime="prof_liberal", sem checar a profissão —
	// mais amplo que o que a lei concede. Por isso continua premissa_tribia na
	// prática, mas não é mais "sem correspondência legal": a correspondência
	// existe e é mais estreita que o modelo assume. Decisão de produto (não
	// tomada nesta auditoria): restringir por profissão, ou manter a
	// aproximação com este limite documentado.
	RegimeProfissionalLiberal = "prof_liberal"
	// RegimeReduzidoZero: aliquota zero — sem tributacao CBS/IBS.
	// Cesta Básica Nacional de Alimentos: Art. 125 + Anexo I da LC 214/2025,
	// instituída pelo Art. 8º da EC 132/2023 — auditado, Onda 2/W1.
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
	// Extinção via revogação dos dispositivos correspondentes das Leis
	// 10.637/2002 e 10.833/2003 — Art. 542, incisos XVIII e XXI da LC 214/2025,
	// vigência 1º/1/2027 (caput do Art. 542) — auditado, Onda 2/W1.
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
//   - 2027-2028: PIS/COFINS extintos; CBS 8,4% (referência 8,5% menos redução
//     compensatória de 0,1 p.p.); IBS nominal em 0,1%.
//   - 2029-2032: IBS sobe 10/20/30/40% da referência (18,5%); ICMS/ISS caem
//     na mesma proporção.
//   - 2033: vigência integral, CBS 8,5% + IBS 18,5% = 27,0% — validado
//     contra a Calculadora oficial da RFB (W7/B2.1, 28/08/2026), ainda
//     pendente de fixação definitiva por Resolução do Senado (ver RuleBasis).
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
// Regimes diferenciados (ver a citação de cada um no comentário da constante
// correspondente, acima — auditados na Onda 2/W1):
//   - RegimeDiferenciado60: 60% de reducao — paga 40% da aliquota padrao.
//   - RegimeProfissionalLiberal: reducao ilustrativa de 30% — paga 70% da aliquota padrao.
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

// meiMonthlyDASBRL: valor mensal ilustrativo do DAS para simulacao em base
// mensal. Congelado como constante (W7/B2.2 — ver o comentário no topo de
// company_regime.go); nao substitui assessoria, nao modela anexo, funcionario
// nem teto de faturamento. Sem fonte legal — o DAS real do MEI e uma fracao
// do salario minimo + parcela fixa de ICMS/ISS, nao um valor fixo em R$; este
// numero e premissa_tribia, nao lei_calendario (RuleBasis, transition_table.go).
const meiMonthlyDASBRLStr = "85"

// MEIMonthlyDAS retem o valor mensal ilustrativo do DAS.
func MEIMonthlyDAS() decimal.Decimal {
	return decimal.RequireFromString(meiMonthlyDASBRLStr)
}
