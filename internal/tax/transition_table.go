package tax

import "github.com/shopspring/decimal"

// RuleBasis declara a proveniência de uma linha da tabela de transição —
// nenhuma linha entra sem isto preenchido (W7/B2.2, docs/roadmap-execucao.md).
//
// Kind:
//   - "lei_calendario": fato fixado no calendário de transição da reforma
//     (extinção de PIS/COFINS em 2027, rampa do IBS/ICMS/ISS em 1/10 por ano
//     entre 2029-2032) — o número do dispositivo que o fixa ainda não foi
//     auditado contra o texto sancionado da LC 214/2025 (o corpus ingerido
//     hoje é o PLP 68/2024 pré-sanção; a auditoria é a Onda 2 do W1).
//   - "estimativa_oficial": a lei delega a fixação da alíquota de referência
//     (LC 214/2025, delegação a Resolução do Senado sobre cálculo do TCU —
//     TODO(W1-onda2): confirmar o artigo exato) — o valor numérico é a
//     projeção calibrada do Ministério da Fazenda, não texto legal.
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
		Basis: RuleBasis{Kind: "lei_calendario", Note: "Fase-teste: CBS 0,9% + IBS 0,1%, compensável com PIS/COFINS; dispensa de recolhimento se cumpridas as obrigações acessórias (não modelado — o motor cobra como custo real, ver README). TODO(W1-onda2): confirmar numeração exata contra o texto sancionado."},
	},
	{
		Year: 2027, PISCOFINSFactor: d("0"), CBSRate: d("0.087"), IBSRate: d("0.001"), ISSFactor: d("1"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "PIS/COFINS extintos (fato do calendário); IBS permanece nominal em 0,1% até 2028 (fato do calendário). CBS entra com a alíquota de referência menos 0,1 p.p. de redução compensatória (8,8% - 0,1% = 8,7%) — o valor exato depende da alíquota de referência, ainda não fixada em lei. TODO(W1-onda2): confirmar numeração."},
	},
	{
		Year: 2028, PISCOFINSFactor: d("0"), CBSRate: d("0.087"), IBSRate: d("0.001"), ISSFactor: d("1"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "Igual a 2027 — a rampa do IBS só começa em 2029 (fato do calendário); o valor de CBS depende da alíquota de referência. TODO(W1-onda2): confirmar numeração."},
	},
	{
		Year: 2029, PISCOFINSFactor: d("0"), CBSRate: d("0.088"), IBSRate: d("0.0177"), ISSFactor: d("0.9"),
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "IBS a 10% da alíquota de referência; ICMS/ISS reduzidos a 90% na mesma proporção — a rampa de 1/10 ao ano é fato do calendário, os valores absolutos dependem da alíquota de referência (ainda não fixada em lei, ver LC 214/2025 — TODO(W1-onda2): confirmar o dispositivo que delega a fixação, provável Art. 349, a uma Resolução do Senado sobre cálculo do TCU)."},
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
		Basis: RuleBasis{Kind: "estimativa_oficial", Note: "Vigência integral — ICMS/ISS extintos é fato do calendário; o split 8,8% CBS + 17,7% IBS (total 26,5%) é a projeção oficial do MF/TCU, ainda pendente de fixação por Resolução do Senado. TODO(W1-onda2): confirmar o dispositivo de delegação."},
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
