package tax

import "github.com/shopspring/decimal"

// Service representa uma receita de saída da empresa (serviço prestado).
type Service struct {
	ID          string
	Description string
	Amount      decimal.Decimal // valor bruto do serviço
	ISSRate     decimal.Decimal // alíquota ISS vigente (ex: 0.05 = 5%)
	// RegimeType define o regime tributário do serviço na LC 214/2025.
	// Afeta a alíquota efetiva de CBS/IBS no cenário projetado.
	// Valores válidos: RegimePadrao, RegimeDiferenciado60, RegimeReduzidoZero.
	RegimeType string
}

// Expense representa uma despesa que pode gerar crédito tributário.
// IsEligible é definido pelo usuário ou classificado pelo RAG.
// RegimeType é o regime tributário do fornecedor (ex: "diferenciado_60" para serviços
// de educação/saúde), que determina a alíquota efetiva de crédito CBS/IBS.
type Expense struct {
	ID          string
	Description string
	Amount      decimal.Decimal
	IsEligible  bool
	RegimeType  string
	// LegalBase é a citação do RAG que embasou a classificação desta despesa
	// no classificador (achado 7/PR5, Etapa C) — puro passthrough do que o
	// frontend já recebeu de POST /credit-classifications/batch antes de
	// simular. O motor nunca interpreta nem valida este texto (não tem
	// acesso ao RAG); vazio quando a IA não citou nada — nunca preenchido
	// aqui por invenção.
	LegalBase string
}

// SimulationInput reúne todos os dados necessários para uma simulação.
// Year deve estar entre 2026 e 2033 (período de transição da LC 214/2025).
// CompanyRegime (JSON company_regime):
//   - vazio ou "regular": atual = PIS/COFINS/ISS; projetado = CBS/IBS por serviço + créditos.
//   - "mei": DAS fixo mensal (ilustrativo); atual = projetado; sem CBS/IBS sobre receita.
//   - "simples_puro" / "simples_hibrido": atual = taxa ilustrativa única sobre faturamento
//     (baseline Simples; ver SimplesIllustrativeCurrentRate); projetado puro = IBS/CBS embutidos
//     modelados como alíquota baixa sem créditos; projetado híbrido = mesmo trilho CBS/IBS + créditos que "regular".
//   - "diferenciado_60": perfil setorial (LC 214/2025, Título IV Capítulo II —
//     Art. 126 institui o regime, percentual fixado artigo a artigo por
//     categoria, ex. Art. 130 saúde/Art. 129 educação; auditado, Onda 2/W1 —
//     "arts. 123-124" da versão anterior deste comentário estava errado: são
//     sobre devolução de IBS/CBS a pessoas físicas, tema não relacionado);
//     atual = mesmo que "regular"; projetado aplica alíquota CBS+IBS efetiva = 40% da padrão do ano (redução de 60%);
//     slug em company_regime.go (CompanyRegimeSectorDiferenciado60) e rules.go (RegimeDiferenciado60).
//     Créditos seguem regime_type de cada despesa.
//   - "aliquota_zero": perfil cesta básica / social (LC 214/2025, Art. 125 + Anexo I, Cesta Básica
//     Nacional de Alimentos instituída pelo Art. 8º da EC 132/2023 — auditado, Onda 2/W1; "Art. 120"
//     da versão anterior deste comentário estava errado); atual = mesmo
//     que "regular"; projetado força CBS+IBS zero em toda a receita de serviços (rules.RegimeReduzidoZero na saída);
//     créditos seguem regime_type de cada despesa (líquido projetado pode ser negativo = posição de crédito).
//   - "imobiliario_venda": atual = regular; projetado = max(0, receita total − ImobiliarioRedutorAjusteBRL) × (alíquota padrão do ano × 60%) — ilustrativo.
//   - "imobiliario_aluguel": idem com multiplicador 40% na alíquota padrão (redução de 60% sobre a alíquota).
//   - "prof_liberal": atual = regular; projetado aplica alíquota CBS+IBS efetiva = 70% da padrão do ano (redução ilustrativa de 30%);
//     slug em company_regime.go e rules.go (RegimeProfissionalLiberal). Créditos seguem regime_type de cada despesa.
//   - "exportadora": atual = regular; projetado força CBS+IBS zero em toda a receita (imunidade ilustrativa na
//     saída). Auditoria da Onda 2/W1: "Art. 52 LC 214/2025" da versão anterior deste comentário estava
//     errado — o Art. 52 é regra genérica ("operações sujeitas a alíquota zero mantêm os créditos"), não
//     específica de exportação. A imunidade de exportação em si não tem artigo próprio no corpus da LC 214
//     ingerido — é imunidade constitucional (Art. 156-A CF/88, incluído pela EC 132/2023), não verificada
//     nesta auditoria (fora do corpus). O único ponto confirmado no texto da LC 214 é o Art. 51, § 2º, I, que
//     exclui exportações da regra de anulação de créditos aplicável a operações imunes/isentas — evidência
//     indireta de que a imunidade já existe em outro lugar, não uma fonte primária;
//     créditos seguem regime_type de cada despesa; líquido projetado frequentemente negativo (saldo credor ilustrativo). Distinto de "aliquota_zero" na narrativa de produto.
//   - "entidade_imune": atual = regular (baseline ilustrativo; não modela imunidade integral no legado); projetado CBS+IBS zero na saída e
//     créditos projetados zerados (sem apropriação no modelo — consumidor final ilustrativo); NetTax projetado típico = 0. Distinto de "exportadora" (lá há créditos).
//
// CompanyContext: texto livre; o ramo MEI exige company_regime "mei".
type SimulationInput struct {
	Year                        int
	CompanyRegime               string
	CompanyContext              string
	Services                    []Service
	Expenses                    []Expense
	ImobiliarioRedutorAjusteBRL decimal.Decimal // redutor de base em R$ na projeção imobiliária; 0 se omitido
}

// TaxComponents decompõe o bruto de um TaxBreakdown por tributo — insumo
// interno para comparar o motor componente a componente contra a Calculadora
// de Tributos da RFB (W7/B2.1). Cada campo é a fatia bruta daquele tributo,
// arredondada a 2 casas de forma independente (como qualquer linha monetária);
// por isso a soma dos campos pode divergir do GrossTax agregado em até um
// centavo por tributo — GrossTax continua sendo o valor autoritativo, nunca
// recalculado a partir daqui (ver TestTaxComponents_SomaReproduzGrossTax).
//
// Regime atual: só PIS, COFINS e ISS têm valor; CBS/IBS ficam zerados.
// Regime projetado: só CBS e IBS têm valor; PIS/COFINS/ISS ficam zerados.
// Nenhum regime usa os dois blocos ao mesmo tempo.
type TaxComponents struct {
	PIS    decimal.Decimal
	COFINS decimal.Decimal
	ISS    decimal.Decimal
	CBS    decimal.Decimal
	IBS    decimal.Decimal
}

// Sum soma os cinco tributos — comparar com GrossTax é a invariante testada.
func (c TaxComponents) Sum() decimal.Decimal {
	return c.PIS.Add(c.COFINS).Add(c.ISS).Add(c.CBS).Add(c.IBS)
}

// TaxBreakdown detalha os componentes de um cenário tributário.
type TaxBreakdown struct {
	GrossTax   decimal.Decimal // imposto sobre a saída (receita)
	Credits    decimal.Decimal // créditos sobre a entrada elegível
	NetTax     decimal.Decimal // imposto líquido = GrossTax - Credits
	Components TaxComponents   // decomposição por tributo (W7/B2.1) — ver TaxComponents
	// Trace é a memória de cálculo deste cenário — os passos ordenados que
	// produziram GrossTax/Credits/NetTax, item a item (W2/PR1,
	// docs/roadmap-execucao.md, Etapa C). Current e Projected têm cada um o
	// seu próprio Trace, nunca misturados — mesma separação de TaxComponents.
	// Todo ramo de Calculate preenche um Trace não-vazio.
	Trace []CalculationStep
}

// SimulationResult compara o regime atual (PIS/COFINS/ISS) com o projetado (IBS/CBS).
// Delta = Projected.NetTax − Current.NetTax: positivo = custo adicional; negativo = economia.
type SimulationResult struct {
	Year int
	// Regime é o ramo de Calculate que produziu este resultado — um dos
	// CompanyRegime* de company_regime.go (ou CompanyRegimeRegular para
	// entrada vazia/"regular"). Sem isto (W2/PR1, achado 2 da Etapa C), quem
	// lê o resultado não sabia por qual caminho de cálculo ele passou — e os
	// ramos usam fórmulas diferentes, então "refazer a conta" era impossível
	// sem essa informação.
	Regime    string
	Current   TaxBreakdown    // regime atual
	Projected TaxBreakdown    // regime projetado (IBS + CBS)
	Delta     decimal.Decimal // Projected.NetTax - Current.NetTax
	DeltaPct  decimal.Decimal // Delta / Current.NetTax * 100 (quando Current > 0)
}
