package tax

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/shopspring/decimal"
)

// RuleBasis declara a proveniência de uma linha da tabela de transição —
// nenhuma linha entra sem isto preenchido (W7/B2.2, docs/roadmap-execucao.md).
//
// Kind:
//   - "lei_calendario": fato fixado no calendário de transição da reforma —
//     numeração AUDITADA contra o texto compilado da LC 214/2025 ingerido na
//     Onda 2/W1 (docs/lc214_2025_limpa.md, 626 chunks; PLP 68/2024 pré-sanção
//     não é mais a única referência).
//   - "estimativa_oficial": a lei delega a fixação da alíquota de referência
//     ao Senado Federal com base em cálculo do TCU — LC 214/2025, Art. 349,
//     caput (confirmado na auditoria, Onda 2/W1: inciso I fixa a delegação da
//     CBS 2027-2033; inciso II a do IBS estadual/municipal 2029-2033; inciso
//     III a do redutor sobre operações da administração pública 2027-2033).
//     O valor numérico usado aqui é a projeção calibrada do MF/TCU, não texto
//     legal — a lei só fixa QUEM decide e COMO, não o número final.
//   - "premissa_tribia": modelagem do produto, sem correspondência legal
//     directa (ex.: redução ilustrativa de profissões liberais).
type RuleBasis struct {
	Kind string
	Note string
}

// TransitionYear é uma linha da tabela de transição — um ano completo com
// proveniência declarada. Ver RulesForYear e ISSMunicipalTransitionFactor,
// que leem esta tabela em vez de repetir os literais.
type TransitionYear struct {
	Year int

	// PISCOFINSFactor: fator de manutenção sobre PIS/COFINS plenos (1 = pleno; 0 = extinto).
	PISCOFINSFactor decimal.Decimal
	// CBSRate / IBSRate: alíquotas efetivas do ano.
	CBSRate decimal.Decimal
	IBSRate decimal.Decimal
	// ISSFactor: fator sobre a alíquota de ISS informada no input (1 = integral; 0 = extinto).
	ISSFactor decimal.Decimal

	Basis RuleBasis
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// transitionTable é a tabela de transição 2026-2033, corrigida contra o
// calendário legal (W7/B2.2 — ver docs/roadmap-execucao.md para o
// levantamento que motivou a correção e o diff desta PR contra a anterior).
//
// Alíquota de referência assumida: CBS 8,8% + IBS 17,7% = 26,5% (projeção
// oficial do MF/TCU — ainda não fixada em lei, ver RuleBasis "estimativa_oficial"
// em cada linha 2027-2033). O total 26,5% não muda em relação à versão
// anterior desta tabela; o que muda é como CBS e IBS se dividem ano a ano.
var transitionTable = []TransitionYear{
	{
		Year: 2026, PISCOFINSFactor: d("1"), CBSRate: d("0.009"), IBSRate: d("0.001"), ISSFactor: d("1"),
		Basis: RuleBasis{Kind: "lei_calendario", Note: "Fase-teste: CBS 0,9% fixada no Art. 346 (fato gerador 01/01 a 31/12/2026); IBS 0,1% fixada no Art. 343 — texto legal literal, não estimativa, os dois auditados contra o compilado (Onda 2/W1). Compensável com PIS/COFINS; dispensa de recolhimento se cumpridas as obrigações acessórias (não modelado — o motor cobra como custo real, ver README)."},
	},
	{
		Year: 2027, PISCOFINSFactor: d("0"), CBSRate: d("0.087"), IBSRate: d("0.001"), ISSFactor: d("1"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "PIS/COFINS extintos por revogação dos dispositivos correspondentes das Leis 10.637/2002 e 10.833/2003 (Art. 542, incisos XVIII e XXI, vigência 1º/1/2027 — caput do Art. 542; auditado, Onda 2/W1). IBS FIXADO EM LEI em 0,1% (Art. 344: 0,05% estadual + 0,05% municipal — não é estimativa, apesar do Kind desta linha). CBS entra com a alíquota de referência menos 0,1 p.p. de redução compensatória (Art. 347) — só esta parcela depende da alíquota de referência, ainda não fixada (Art. 349)."},
	},
	{
		Year: 2028, PISCOFINSFactor: d("0"), CBSRate: d("0.087"), IBSRate: d("0.001"), ISSFactor: d("1"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "Igual a 2027 (mesmos Art. 344/347/542 — a rampa do IBS só começa em 2029, Art. 349); só o valor de CBS depende da alíquota de referência ainda não fixada."},
	},
	{
		Year: 2029, PISCOFINSFactor: d("0"), CBSRate: d("0.088"), IBSRate: d("0.0177"), ISSFactor: d("0.9"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "IBS a 10% da alíquota de referência; ICMS/ISS reduzidos a 90% na mesma proporção. Delegação da fixação ao Senado Federal, com base em cálculo do TCU: Art. 349, caput, incisos I a III (auditado, Onda 2/W1 — não é mais \"provável\"). A rampa de 1/10 ao ano em si é calendário constitucional (EC 132/2023): a LC 214 remete a ela como \"transição prevista nos arts. 124 a 133 do ADCT\" sem repetir os percentuais — o inciso exato do ADCT que fixa 90/80/70/60% não foi verificado nesta auditoria (fora do corpus ingerido, que é só a LC 214)."},
	},
	{
		Year: 2030, PISCOFINSFactor: d("0"), CBSRate: d("0.088"), IBSRate: d("0.0354"), ISSFactor: d("0.8"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "IBS a 20% da alíquota de referência; ICMS/ISS reduzidos a 80%. Mesma ressalva de 2029."},
	},
	{
		Year: 2031, PISCOFINSFactor: d("0"), CBSRate: d("0.088"), IBSRate: d("0.0531"), ISSFactor: d("0.7"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "IBS a 30% da alíquota de referência; ICMS/ISS reduzidos a 70%. Mesma ressalva de 2029."},
	},
	{
		Year: 2032, PISCOFINSFactor: d("0"), CBSRate: d("0.088"), IBSRate: d("0.0708"), ISSFactor: d("0.6"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "IBS a 40% da alíquota de referência; ICMS/ISS reduzidos a 60%. Mesma ressalva de 2029."},
	},
	{
		Year: 2033, PISCOFINSFactor: d("0"), CBSRate: d("0.088"), IBSRate: d("0.177"), ISSFactor: d("0"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "Vigência integral — ICMS/ISS extintos é fato do calendário; o split 8,8% CBS + 17,7% IBS (total 26,5%) é a projeção oficial do MF/TCU, ainda pendente de fixação por Resolução do Senado nos termos do Art. 349 (auditado, Onda 2/W1)."},
	},
}

func transitionRow(year int) TransitionYear {
	if year < 2026 {
		year = 2026
	}
	if year > 2033 {
		year = 2033
	}
	for _, row := range transitionTable {
		if row.Year == year {
			return row
		}
	}
	// Inatingível: transitionTable cobre 2026-2033 e year já foi clampado acima;
	// TestTransitionTable_CobreTodosOsAnos garante isso na suíte.
	panic("tax: ano de transição sem linha na tabela")
}

// TransitionYearBasis devolve a proveniência da linha da tabela usada no ano
// (clampado a 2026-2033, mesma regra de RulesForYear).
func TransitionYearBasis(year int) RuleBasis {
	return transitionRow(year).Basis
}

// TransitionTableHash resume os campos numéricos da tabela de transição
// (Year, PISCOFINSFactor, CBSRate, IBSRate, ISSFactor — não Basis, que é só
// documentação) num hash estável.
//
// internal/enginevalidation.Build carimba este valor na evidência gravada
// por internal/tax/rfb_cross_test.go e o recalcula a cada leitura: se a
// tabela mudar depois que a validação contra a Calculadora RFB foi rodada,
// o selo deixa de afirmar validação — a evidência é sobre os números de
// ENTÃO, e ficaria sustentando uma afirmação sobre uma tabela que já não
// existe (W7/B2.3, docs/roadmap-execucao.md).
func TransitionTableHash() string {
	h := sha256.New()
	for _, row := range transitionTable {
		fmt.Fprintf(h, "%d|%s|%s|%s|%s\n",
			row.Year, row.PISCOFINSFactor.String(), row.CBSRate.String(), row.IBSRate.String(), row.ISSFactor.String())
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
