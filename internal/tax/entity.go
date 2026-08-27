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
}

// SimulationInput reúne todos os dados necessários para uma simulação.
// Year deve estar entre 2026 e 2033 (período de transição da LC 214/2025).
// CompanyRegime (JSON company_regime):
//   - vazio ou "regular": atual = PIS/COFINS/ISS; projetado = CBS/IBS por serviço + créditos.
//   - "mei": DAS fixo mensal (ilustrativo); atual = projetado; sem CBS/IBS sobre receita.
//   - "simples_puro" / "simples_hibrido": atual = taxa ilustrativa única sobre faturamento
//     (baseline Simples; ver SimplesIllustrativeCurrentRate); projetado puro = IBS/CBS embutidos
//     modelados como alíquota baixa sem créditos; projetado híbrido = mesmo trilho CBS/IBS + créditos que "regular".
//   - "diferenciado_60": perfil setorial (LC 214/2025, arts. 123–124 e regime diferenciado — TODO W1-onda2: confirmar numeração); atual = mesmo
//     que "regular"; projetado aplica alíquota CBS+IBS efetiva = 40% da padrão do ano (redução de 60%);
//     slug em company_regime.go (CompanyRegimeSectorDiferenciado60) e rules.go (RegimeDiferenciado60).
//     Créditos seguem regime_type de cada despesa.
//   - "aliquota_zero": perfil cesta básica / social (LC 214/2025, Art. 120 e Anexo I — ilustrativo, TODO W1-onda2: confirmar numeração); atual = mesmo
//     que "regular"; projetado força CBS+IBS zero em toda a receita de serviços (rules.RegimeReduzidoZero na saída);
//     créditos seguem regime_type de cada despesa (líquido projetado pode ser negativo = posição de crédito).
//   - "imobiliario_venda": atual = regular; projetado = max(0, receita total − ImobiliarioRedutorAjusteBRL) × (alíquota padrão do ano × 60%) — ilustrativo.
//   - "imobiliario_aluguel": idem com multiplicador 40% na alíquota padrão (redução de 60% sobre a alíquota).
//   - "prof_liberal": atual = regular; projetado aplica alíquota CBS+IBS efetiva = 70% da padrão do ano (redução ilustrativa de 30%);
//     slug em company_regime.go e rules.go (RegimeProfissionalLiberal). Créditos seguem regime_type de cada despesa.
//   - "exportadora": atual = regular; projetado força CBS+IBS zero em toda a receita (imunidade ilustrativa na saída, ex. Art. 52 LC 214/2025 — TODO W1-onda2: confirmar numeração);
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
}

// SimulationResult compara o regime atual (PIS/COFINS/ISS) com o projetado (IBS/CBS).
// Delta = Projected.NetTax − Current.NetTax: positivo = custo adicional; negativo = economia.
type SimulationResult struct {
	Year      int
	Current   TaxBreakdown    // regime atual
	Projected TaxBreakdown    // regime projetado (IBS + CBS)
	Delta     decimal.Decimal // Projected.NetTax - Current.NetTax
	DeltaPct  decimal.Decimal // Delta / Current.NetTax * 100 (quando Current > 0)
}
