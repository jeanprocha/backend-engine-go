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

// transitionTable é a tabela de transição 2026-2033, corrigida contra a
// Calculadora oficial da RFB (W7/B2.1, docs/roadmap-execucao.md — validação
// executada 28/08/2026 contra a API pública hospedada no ambiente de
// homologação do piloto, piloto-cbs.tributos.gov.br, versão do app 1.3.0,
// base de dados V0042/2026-07-07; endpoints dados-abertos/aliquota-uniao,
// aliquota-uf, aliquota-municipio).
//
// Alíquota de referência aplicada pela Calculadora oficial: CBS 8,5% + IBS
// 18,5% (16,0% UF + 2,5% município, verificado idêntico em RS/Porto Alegre
// e SP/São Paulo — a variação por ente ainda não está em vigor na
// transição) = 27,0% em regime pleno. Versão anterior desta tabela assumia
// CBS 8,8% + IBS 17,7% = 26,5% (uma estimativa própria do TribIA, nunca
// conferida contra o motor oficial) — a estrutura da rampa (10/20/30/40%
// para 2029-2032, integral em 2033; -0,1 p.p. de redução compensatória só
// em 2027-2028) não mudou, só o valor da referência.
//
// Achado durante a validação, não corrigido nesta linha: a LC 214/2025,
// Art. 475, §§ 9º-11 (avaliação quinquenal, docs/lc214_2025_limpa.md)
// estabelece que se a soma das alíquotas de referência estimadas para 2033
// superar 26,5%, o Executivo deve enviar projeto de lei complementar
// propondo reduzi-la a esse teto. Os 27,0% que a Calculadora aplica HOJE
// já excedem esse teto legal em 0,5 p.p. — não é contradição (a avaliação
// quinquenal do § 9º só ocorre com base em dados de 2030, com efeito a
// partir de 2032/2033), mas é um sinal concreto de que este número pode
// mudar por força de lei antes da vigência plena. TransitionTableHash
// existe exatamente para isso: se a referência mudar, a evidência gravada
// contra o valor antigo deixa de sustentar o selo.
var transitionTable = []TransitionYear{
	{
		Year: 2026, PISCOFINSFactor: d("1"), CBSRate: d("0.009"), IBSRate: d("0.001"), ISSFactor: d("1"),
		Basis: RuleBasis{Kind: "lei_calendario", Note: "Fase-teste: CBS 0,9% fixada no Art. 346 (fato gerador 01/01 a 31/12/2026); IBS 0,1% fixada no Art. 343 — texto legal literal, não estimativa, os dois foram conferidos contra o texto da lei e batem exatamente com a Calculadora oficial da RFB (validação de 28/08/2026 — único ano sem divergência). Compensável com PIS/COFINS; há dispensa de recolhimento se cumpridas as obrigações acessórias (não modelado — o motor cobra como custo real)."},
	},
	{
		Year: 2027, PISCOFINSFactor: d("0"), CBSRate: d("0.084"), IBSRate: d("0.001"), ISSFactor: d("1"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "PIS/COFINS extintos por revogação dos dispositivos correspondentes das Leis 10.637/2002 e 10.833/2003 (Art. 542, incisos XVIII e XXI, vigência 1º/1/2027 — caput do Art. 542). IBS FIXADO EM LEI em 0,1% (Art. 344: 0,05% estadual + 0,05% municipal — não é estimativa, apesar do Kind desta linha). CBS = alíquota de referência (8,5%, conforme a Calculadora oficial da RFB, validação de 28/08/2026) menos 0,1 p.p. de redução compensatória (Art. 347) = 8,4%; a delegação da fixação está no Art. 349."},
	},
	{
		Year: 2028, PISCOFINSFactor: d("0"), CBSRate: d("0.084"), IBSRate: d("0.001"), ISSFactor: d("1"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "Igual a 2027 (mesmos Art. 344/347/542 — a rampa do IBS só começa em 2029, Art. 349); CBS = 8,5% (alíquota de referência, validada contra a Calculadora oficial da RFB) − 0,1 p.p. = 8,4%."},
	},
	{
		Year: 2029, PISCOFINSFactor: d("0"), CBSRate: d("0.085"), IBSRate: d("0.0185"), ISSFactor: d("0.9"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "IBS a 10% da alíquota de referência (18,5% = 16,0% UF + 2,5% município, conforme a Calculadora oficial da RFB, validado em 28/08/2026 — a API oficial já devolve o valor rampeado por data, conferido igual em duas UFs/municípios distintos); ICMS/ISS reduzidos a 90% na mesma proporção. Delegação da fixação ao Senado Federal, com base em cálculo do TCU: Art. 349, caput, incisos I a III. A rampa de 1/10 ao ano em si é calendário constitucional (EC 132/2023): a LC 214 remete a ela como \"transição prevista nos arts. 124 a 133 do ADCT\" sem repetir os percentuais — o inciso exato do ADCT que fixa 90/80/70/60% não foi verificado (está fora do texto da LC 214, a única lei consultada aqui), mas os PERCENTUAIS de rampa (10/20/30/40%) foram confirmados contra a resposta da Calculadora oficial."},
	},
	{
		Year: 2030, PISCOFINSFactor: d("0"), CBSRate: d("0.085"), IBSRate: d("0.0370"), ISSFactor: d("0.8"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "IBS a 20% da alíquota de referência (18,5%, validada contra a Calculadora oficial da RFB); ICMS/ISS reduzidos a 80%. Mesma ressalva de 2029."},
	},
	{
		Year: 2031, PISCOFINSFactor: d("0"), CBSRate: d("0.085"), IBSRate: d("0.0555"), ISSFactor: d("0.7"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "IBS a 30% da alíquota de referência (18,5%, validada contra a Calculadora oficial da RFB); ICMS/ISS reduzidos a 70%. Mesma ressalva de 2029."},
	},
	{
		Year: 2032, PISCOFINSFactor: d("0"), CBSRate: d("0.085"), IBSRate: d("0.0740"), ISSFactor: d("0.6"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "IBS a 40% da alíquota de referência (18,5%, validada contra a Calculadora oficial da RFB); ICMS/ISS reduzidos a 60%. Mesma ressalva de 2029."},
	},
	{
		Year: 2033, PISCOFINSFactor: d("0"), CBSRate: d("0.085"), IBSRate: d("0.185"), ISSFactor: d("0"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "Vigência integral — ICMS/ISS extintos é fato do calendário; o split 8,5% CBS + 18,5% IBS (total 27,0%) é o que a Calculadora oficial da RFB aplica hoje como referência (validado em 28/08/2026 — API pública do piloto, versão 1.3.0/base V0042), ainda pendente de fixação definitiva por Resolução do Senado nos termos do Art. 349. Ressalva relevante: o Art. 475, § 11 estabelece um teto de 26,5% para essa soma, sujeito a correção por lei complementar na avaliação quinquenal de 2030 (§§ 9º-10) — os 27,0% atuais já excedem esse teto; este número tem chance concreta de mudar antes da vigência plena."},
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
