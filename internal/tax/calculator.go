package tax

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// Engine define o contrato do motor de simulação tributária.
// Ao depender da interface (e não da struct concreta), a camada HTTP pode
// receber um mock nos testes sem acessar banco ou fazer cálculos reais.
type Engine interface {
	Calculate(ctx context.Context, input SimulationInput) (SimulationResult, error)
}

// calculator é a implementação concreta de Engine.
type calculator struct{}

// NewCalculator retorna a implementação padrão do motor de cálculo.
func NewCalculator() Engine {
	return &calculator{}
}

// Calculate executa a simulação comparativa entre regime atual e projetado.
//
// Regime atual: PIS + COFINS (não-cumulativo) + ISS, aplicados sobre receita.
// Regime projetado: CBS + IBS sobre receita, com crédito integral sobre despesas elegíveis.
//
// Premissa: ISS não gera crédito no regime atual (cumulativo para serviços simples).
// CBS e IBS admitem crédito pleno sobre insumos elegíveis (não-cumulatividade ampla).
func (c *calculator) Calculate(_ context.Context, input SimulationInput) (SimulationResult, error) {
	if len(input.Services) == 0 {
		return SimulationResult{}, fmt.Errorf("calculator: nenhum servico informado")
	}
	// W7/B2.2: um motor que devolve número plausível para entrada que não
	// entende não pode carregar selo de validação (B2.3). O handler HTTP já
	// valida year e não valida company_regime — aqui é defesa em profundidade
	// para qualquer chamador direto do pacote (testes, futuras integrações).
	if input.Year < 2026 || input.Year > 2033 {
		return SimulationResult{}, fmt.Errorf("calculator: year %d fora do intervalo suportado (2026-2033)", input.Year)
	}
	if !IsKnownCompanyRegime(input.CompanyRegime) {
		return SimulationResult{}, fmt.Errorf("calculator: company_regime %q desconhecido", input.CompanyRegime)
	}

	// MEI: premissa mensal com DAS fixo (ilustrativo); não incide CBS/IBS nem PIS/COFINS/ISS da receita.
	// Ativa apenas com company_regime "mei".
	if isMEISimulation(input.CompanyRegime, input.CompanyContext) {
		fixed := MEIMonthlyDAS().Round(2)
		t := &calcTrace{}
		t.add("", "DAS mensal (MEI)",
			"constante ilustrativa (MEIMonthlyDAS) — sem base legal, não modela anexo, funcionário nem teto de faturamento",
			fixed, true)
		// Components não preenchido: o DAS é um valor único ilustrativo, sem
		// decomposição PIS/COFINS/ISS/CBS/IBS natural — inventar uma alocação
		// seria fabricar um número que a lei não separa.
		return SimulationResult{
			Year:   input.Year,
			Regime: CompanyRegimeMEI,
			Current: TaxBreakdown{
				GrossTax: fixed,
				Credits:  decimal.Zero,
				NetTax:   fixed,
				Trace:    t.steps,
			},
			Projected: TaxBreakdown{
				GrossTax: fixed,
				Credits:  decimal.Zero,
				NetTax:   fixed,
				Trace:    t.steps,
			},
			Delta:    decimal.Zero,
			DeltaPct: decimal.Zero,
		}, nil
	}

	rules := RulesForYear(input.Year)

	totalRevenue, err := sumServiceRevenue(input.Services)
	if err != nil {
		return SimulationResult{}, err
	}

	// Simples Nacional (puro vs híbrido): baseline atual ilustrativo sobre faturamento;
	// projetado = alíquota baixa sem créditos (puro) ou CBS/IBS pleno com créditos (híbrido).
	if IsSimplesNationalProfile(input.CompanyRegime) {
		if err := validateExpensesNonNegative(input.Expenses); err != nil {
			return SimulationResult{}, err
		}
		illustrative := SimplesIllustrativeCurrentRate()
		currentGross := totalRevenue.Mul(illustrative).Round(2)
		currentTrace := &calcTrace{}
		currentTrace.add("", "Bruto do regime atual (Simples, ilustrativo)",
			"receita total × taxa ilustrativa sobre faturamento", currentGross, true,
			in("receita_total", totalRevenue), in("taxa_ilustrativa", illustrative))
		currentTrace.add("", "Líquido do regime atual",
			"bruto − créditos (Simples não gera crédito no atual, ilustrativo)", currentGross, true,
			in("bruto", currentGross), in("creditos", decimal.Zero))
		// Components não preenchido: a taxa ilustrativa do Simples é um número
		// único sobre o faturamento, sem decomposição PIS/COFINS/ISS natural.
		current := TaxBreakdown{
			GrossTax: currentGross,
			Credits:  decimal.Zero,
			NetTax:   currentGross,
			Trace:    currentTrace.steps,
		}
		var projected TaxBreakdown
		regime := CompanyRegimeSimplesHibrido
		if strings.EqualFold(strings.TrimSpace(input.CompanyRegime), CompanyRegimeSimplesPuro) {
			regime = CompanyRegimeSimplesPuro
			embeddedRate := SimplesPuroEffectiveIBSCBSRate()
			pg := totalRevenue.Mul(embeddedRate).Round(2)
			projectedTrace := &calcTrace{}
			projectedTrace.add("", "Bruto do regime projetado (Simples puro, ilustrativo)",
				"receita total × alíquota IBS/CBS embutida no DAS", pg, true,
				in("receita_total", totalRevenue), in("aliquota_embutida", embeddedRate))
			projectedTrace.add("", "Líquido do regime projetado",
				"bruto − créditos (Simples puro tem crédito restrito, ilustrativo)", pg, true,
				in("bruto", pg), in("creditos", decimal.Zero))
			// Components não preenchido: mesmo motivo do atual — número único
			// embutido no DAS, sem decomposição CBS/IBS natural.
			projected = TaxBreakdown{GrossTax: pg, Credits: decimal.Zero, NetTax: pg, Trace: projectedTrace.steps}
		} else {
			projected = computeProjectedCBSIBS(rules, input.Services, input.Expenses)
		}
		return finalizeResult(input.Year, regime, current, projected), nil
	}

	// Perfil cesta básica / social: atual = regular; projetado força alíquota CBS+IBS zero em toda a receita
	// (EffectiveProjectedRate(RegimeReduzidoZero)); créditos por despesa — líquido projetado pode ser negativo.
	if IsAliquotaZeroProfile(input.CompanyRegime) {
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		projected := computeProjectedCBSIBSForcedOutputRegime(rules, input.Services, input.Expenses, RegimeReduzidoZero)
		return finalizeResult(input.Year, CompanyRegimeAliquotaZero, current, projected), nil
	}

	// Perfil exportadora (ilustrativo): imunidade integral CBS+IBS na saída; créditos nas compras — alíquota efetiva zero na receita (mesma conta que cesta básica na projeção).
	if IsExportadoraProfile(input.CompanyRegime) {
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		projected := computeProjectedCBSIBSForcedOutputRegime(rules, input.Services, input.Expenses, RegimeReduzidoZero)
		return finalizeResult(input.Year, CompanyRegimeExportadora, current, projected), nil
	}

	// Perfil entidade imune (ilustrativo): saída CBS+IBS zero; sem créditos no projetado (consumidor final no modelo).
	if IsEntidadeImuneProfile(input.CompanyRegime) {
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		gross, components, steps := projectedGrossCBSIBSForcedOutputRegime(rules, input.Services, RegimeReduzidoZero)
		t := &calcTrace{steps: steps}
		t.add("", "Créditos do regime projetado",
			"entidade imune não aproprai créditos no modelo (consumidor final ilustrativo) — forçado a zero",
			decimal.Zero, true)
		t.add("", "Líquido do regime projetado", "bruto − créditos", gross, true,
			in("bruto", gross), in("creditos", decimal.Zero))
		projected := TaxBreakdown{
			GrossTax:   gross,
			Credits:    decimal.Zero,
			NetTax:     gross,
			Components: components,
			Trace:      t.steps,
		}
		return finalizeResult(input.Year, CompanyRegimeEntidadeImune, current, projected), nil
	}

	// Perfil imobiliário: atual = regular; projetado = max(0, receita total − redutor) × (alíquota padrão × fator).
	if IsImobiliarioProfile(input.CompanyRegime) {
		if err := validateExpensesNonNegative(input.Expenses); err != nil {
			return SimulationResult{}, err
		}
		if input.ImobiliarioRedutorAjusteBRL.IsNegative() {
			return SimulationResult{}, fmt.Errorf("calculator: redutor imobiliário não pode ser negativo")
		}
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		mult := imobiliarioStandardRateMultiplier(input.CompanyRegime)
		projected := computeProjectedImobiliario(rules, totalRevenue, input.Expenses, mult, input.ImobiliarioRedutorAjusteBRL)
		regime := CompanyRegimeImobiliarioAluguel
		if IsImobiliarioVendaProfile(input.CompanyRegime) {
			regime = CompanyRegimeImobiliarioVenda
		}
		return finalizeResult(input.Year, regime, current, projected), nil
	}

	// Perfil profissões regulamentadas (ilustrativo): atual = regular; projetado força 70% da alíquota padrão CBS+IBS (redução ilustrativa de 30%).
	if IsProfissionalLiberalProfile(input.CompanyRegime) {
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		projected := computeProjectedCBSIBSForcedOutputRegime(rules, input.Services, input.Expenses, RegimeProfissionalLiberal)
		return finalizeResult(input.Year, CompanyRegimeProfissionalLiberal, current, projected), nil
	}

	// Perfil TribIA — saúde/educação/cultura: saída com redução de 60% na alíquota CBS+IBS (efetiva =
	// rules.CombinedProjectedRate() × 0,4 via EffectiveProjectedRate(RegimeDiferenciado60)).
	// Atual = regular; projetado força esse regime em toda a receita de serviços; créditos por despesa.
	if IsSectorDiferenciado60Profile(input.CompanyRegime) {
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		projected := computeProjectedCBSIBSForcedOutputRegime(rules, input.Services, input.Expenses, RegimeDiferenciado60)
		return finalizeResult(input.Year, CompanyRegimeSectorDiferenciado60, current, projected), nil
	}

	current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
	if err != nil {
		return SimulationResult{}, err
	}
	projected := computeProjectedCBSIBS(rules, input.Services, input.Expenses)

	return finalizeResult(input.Year, CompanyRegimeRegular, current, projected), nil
}

func sumServiceRevenue(services []Service) (decimal.Decimal, error) {
	total := decimal.Zero
	for _, svc := range services {
		if svc.Amount.IsNegative() {
			return decimal.Zero, fmt.Errorf("calculator: servico %q com valor negativo", svc.ID)
		}
		total = total.Add(svc.Amount)
	}
	return total, nil
}

func validateExpensesNonNegative(expenses []Expense) error {
	for _, exp := range expenses {
		if exp.Amount.IsNegative() {
			return fmt.Errorf("calculator: despesa %q com valor negativo", exp.ID)
		}
	}
	return nil
}

// computeCurrentRegularLegacy aplica PIS+COFINS+ISS no bruto atual e créditos de PIS/COFINS sobre despesas elegíveis.
func computeCurrentRegularLegacy(rules TaxRules, totalRevenue decimal.Decimal, services []Service, expenses []Expense) (TaxBreakdown, error) {
	t := &calcTrace{}

	// currentGross soma antes de arredondar (comportamento original, intocado);
	// pisGross/cofinsGross/issGross arredondam cada um por si só, só para
	// TaxComponents (W7/B2.1) — por isso a soma dos três pode divergir do
	// currentGross agregado em até um centavo (ver doc de TaxComponents).
	currentGrossPreISS := totalRevenue.Mul(rules.CombinedCurrentRate())
	t.add("", "PIS+COFINS sobre a receita", "receita total × (alíquota PIS + alíquota COFINS)", currentGrossPreISS, false,
		in("receita_total", totalRevenue), in("aliquota_pis_cofins_combinada", rules.CombinedCurrentRate()))

	issLeg := rules.ISSMunicipalTransitionFactor()
	issUnrounded := decimal.Zero
	for _, svc := range services {
		effectiveISS := svc.ISSRate.Mul(issLeg)
		issSvc := svc.Amount.Mul(effectiveISS)
		issUnrounded = issUnrounded.Add(issSvc)
		t.add(lineItem(svc.ID, svc.Description), "ISS do serviço",
			"valor do serviço × (alíquota ISS informada × fator de transição municipal do ano)", issSvc, false,
			in("valor_servico", svc.Amount), in("aliquota_iss_informada", svc.ISSRate), in("fator_transicao_iss", issLeg))
	}
	currentGross := currentGrossPreISS.Add(issUnrounded).Round(2)
	t.add("", "Bruto do regime atual", "(PIS+COFINS sobre a receita) + (soma do ISS por serviço), arredondado", currentGross, true,
		in("pis_cofins_sobre_receita", currentGrossPreISS), in("iss_total_nao_arredondado", issUnrounded))

	pisGross := totalRevenue.Mul(rules.PISRate).Round(2)
	t.add("", "PIS (componente)",
		"receita total × alíquota PIS — arredondado de forma independente; pode divergir do bruto agregado em até R$ 0,01 (ver TaxComponents)",
		pisGross, true, in("receita_total", totalRevenue), in("aliquota_pis", rules.PISRate))
	cofinsGross := totalRevenue.Mul(rules.COFINSRate).Round(2)
	t.add("", "COFINS (componente)",
		"receita total × alíquota COFINS — mesma ressalva de arredondamento independente",
		cofinsGross, true, in("receita_total", totalRevenue), in("aliquota_cofins", rules.COFINSRate))
	issGross := issUnrounded.Round(2)
	t.add("", "ISS (componente)",
		"soma do ISS por serviço, arredondada — mesma ressalva de arredondamento independente",
		issGross, true, in("iss_total_nao_arredondado", issUnrounded))

	currentCredits := decimal.Zero
	for _, exp := range expenses {
		if exp.IsEligible {
			if exp.Amount.IsNegative() {
				return TaxBreakdown{}, fmt.Errorf("calculator: despesa %q com valor negativo", exp.ID)
			}
			credit := exp.Amount.Mul(rules.CombinedCurrentRate())
			currentCredits = currentCredits.Add(credit)
			t.add(lineItem(exp.ID, exp.Description), "Crédito da despesa (regime atual)",
				"valor da despesa × (alíquota PIS + alíquota COFINS combinada)", credit, false,
				in("valor_despesa", exp.Amount), in("aliquota_pis_cofins_combinada", rules.CombinedCurrentRate()))
		}
	}
	currentCredits = currentCredits.Round(2)
	t.add("", "Créditos do regime atual", "soma dos créditos elegíveis, arredondada", currentCredits, true)
	currentNet := currentGross.Sub(currentCredits).Round(2)
	t.add("", "Líquido do regime atual", "bruto − créditos", currentNet, true,
		in("bruto", currentGross), in("creditos", currentCredits))

	return TaxBreakdown{
		GrossTax: currentGross,
		Credits:  currentCredits,
		NetTax:   currentNet,
		Components: TaxComponents{
			PIS:    pisGross,
			COFINS: cofinsGross,
			ISS:    issGross,
		},
		Trace: t.steps,
	}, nil
}

func computeProjectedCBSIBS(rules TaxRules, services []Service, expenses []Expense) TaxBreakdown {
	return computeProjectedCBSIBSForcedOutputRegime(rules, services, expenses, "")
}

// imobiliarioStandardRateMultiplier: venda paga 60% da alíquota padrão; locação paga 40% (ilustrativo LC 214/2025).
func imobiliarioStandardRateMultiplier(companyRegime string) decimal.Decimal {
	if IsImobiliarioVendaProfile(companyRegime) {
		return decimal.RequireFromString("0.6")
	}
	if IsImobiliarioAluguelProfile(companyRegime) {
		return decimal.RequireFromString("0.4")
	}
	return decimal.NewFromInt(1)
}

// computeProjectedImobiliario aplica CBS+IBS sobre a base (receita total − redutor), truncada em zero; créditos por despesa.
// mult é o multiplicador do perfil (0,6 venda / 0,4 aluguel) — a derivação da
// alíquota efetiva (padrão × mult) mora aqui, não no chamador, para que o
// passo entre no trace junto com o resto do cálculo deste cenário.
func computeProjectedImobiliario(rules TaxRules, totalRevenue decimal.Decimal, expenses []Expense, mult, redutor decimal.Decimal) TaxBreakdown {
	t := &calcTrace{}

	effectiveRate := rules.CombinedProjectedRate().Mul(mult).Round(6)
	t.add("", "Alíquota efetiva (imobiliário)",
		"(CBS+IBS padrão do ano) × multiplicador do perfil (venda=0,6; aluguel=0,4)", effectiveRate, true,
		in("aliquota_padrao_ano", rules.CombinedProjectedRate()), in("multiplicador_perfil", mult))

	taxable := totalRevenue.Sub(redutor)
	if taxable.IsNegative() {
		taxable = decimal.Zero
	}
	t.add("", "Base tributável (imobiliário)", "max(0, receita total − redutor de ajuste)", taxable, false,
		in("receita_total", totalRevenue), in("redutor_ajuste", redutor))

	projectedGross := taxable.Mul(effectiveRate).Round(2)
	t.add("", "Bruto do regime projetado (imobiliário)", "base tributável × alíquota efetiva", projectedGross, true,
		in("base_tributavel", taxable), in("aliquota_efetiva", effectiveRate))

	projectedCredits := decimal.Zero
	for _, exp := range expenses {
		if exp.IsEligible {
			creditRate := rules.EffectiveProjectedRate(exp.RegimeType)
			credit := exp.Amount.Mul(creditRate)
			projectedCredits = projectedCredits.Add(credit)
			t.add(lineItem(exp.ID, exp.Description), "Crédito da despesa (regime projetado)",
				fmt.Sprintf("valor da despesa × alíquota efetiva do regime da despesa (regime: %s)", regimeDisplayLabel(exp.RegimeType)),
				credit, false, in("valor_despesa", exp.Amount), in("aliquota_efetiva", creditRate))
		}
	}
	projectedCredits = projectedCredits.Round(2)
	t.add("", "Créditos do regime projetado", "soma dos créditos elegíveis, arredondada", projectedCredits, true)
	projectedNet := projectedGross.Sub(projectedCredits).Round(2)
	t.add("", "Líquido do regime projetado", "bruto − créditos", projectedNet, true,
		in("bruto", projectedGross), in("creditos", projectedCredits))

	return TaxBreakdown{
		GrossTax: projectedGross,
		Credits:  projectedCredits,
		NetTax:   projectedNet,
		// Components não preenchido: não há decomposição PIS/COFINS/ISS/CBS/IBS
		// natural para a base "receita − redutor" do perfil imobiliário sem
		// inventar uma alocação.
		Trace: t.steps,
	}
}

// projectedGrossCBSIBSForcedOutputRegime soma CBS+IBS sobre a receita de serviços. Se outputRegime não for vazio,
// toda a linha usa EffectiveProjectedRate(outputRegime); senão usa svc.RegimeType por serviço.
// O segundo retorno decompõe o bruto em CBS/IBS (W7/B2.1) — mesma ressalva de arredondamento
// independente de TaxComponents; o primeiro retorno é o valor autoritativo, intocado. O terceiro
// retorno é o trace desta função — quem chama e continua computando (créditos, líquido) estende
// estes passos em vez de descartá-los (W2/PR1).
func projectedGrossCBSIBSForcedOutputRegime(rules TaxRules, services []Service, outputRegime string) (decimal.Decimal, TaxComponents, []CalculationStep) {
	t := &calcTrace{}
	projectedGross := decimal.Zero
	cbsGross := decimal.Zero
	ibsGross := decimal.Zero
	for _, svc := range services {
		rt := svc.RegimeType
		if outputRegime != "" {
			rt = outputRegime
		}
		rate := rules.EffectiveProjectedRate(rt)
		lineGross := svc.Amount.Mul(rate)
		projectedGross = projectedGross.Add(lineGross)
		t.add(lineItem(svc.ID, svc.Description), "CBS+IBS do serviço",
			fmt.Sprintf("valor do serviço × alíquota efetiva (regime: %s)", regimeDisplayLabel(rt)), lineGross, false,
			in("valor_servico", svc.Amount), in("aliquota_efetiva", rate))

		cbsRate, ibsRate := rules.EffectiveProjectedRateSplit(rt)
		cbsGross = cbsGross.Add(svc.Amount.Mul(cbsRate))
		ibsGross = ibsGross.Add(svc.Amount.Mul(ibsRate))
	}
	projectedGross = projectedGross.Round(2)
	t.add("", "Bruto do regime projetado (CBS+IBS)", "soma do CBS+IBS por serviço, arredondada", projectedGross, true)

	cbsGrossR := cbsGross.Round(2)
	t.add("", "CBS (componente)",
		"soma da fatia CBS por serviço, arredondada — mesma ressalva de arredondamento independente de TaxComponents",
		cbsGrossR, true)
	ibsGrossR := ibsGross.Round(2)
	t.add("", "IBS (componente)", "soma da fatia IBS por serviço, arredondada — mesma ressalva", ibsGrossR, true)

	return projectedGross, TaxComponents{CBS: cbsGrossR, IBS: ibsGrossR}, t.steps
}

// computeProjectedCBSIBSForcedOutputRegime calcula CBS/IBS projetado. Se outputRegime não for vazio,
// toda a receita de serviços usa EffectiveProjectedRate(outputRegime); senão usa svc.RegimeType por linha.
// Créditos seguem sempre regime_type de cada despesa.
func computeProjectedCBSIBSForcedOutputRegime(rules TaxRules, services []Service, expenses []Expense, outputRegime string) TaxBreakdown {
	projectedGross, components, steps := projectedGrossCBSIBSForcedOutputRegime(rules, services, outputRegime)
	t := &calcTrace{steps: steps}

	projectedCredits := decimal.Zero
	for _, exp := range expenses {
		if exp.IsEligible {
			creditRate := rules.EffectiveProjectedRate(exp.RegimeType)
			credit := exp.Amount.Mul(creditRate)
			projectedCredits = projectedCredits.Add(credit)
			t.add(lineItem(exp.ID, exp.Description), "Crédito da despesa (regime projetado)",
				fmt.Sprintf("valor da despesa × alíquota efetiva do regime da despesa (regime: %s)", regimeDisplayLabel(exp.RegimeType)),
				credit, false, in("valor_despesa", exp.Amount), in("aliquota_efetiva", creditRate))
		}
	}
	projectedCredits = projectedCredits.Round(2)
	t.add("", "Créditos do regime projetado", "soma dos créditos elegíveis, arredondada", projectedCredits, true)
	projectedNet := projectedGross.Sub(projectedCredits).Round(2)
	t.add("", "Líquido do regime projetado", "bruto − créditos", projectedNet, true,
		in("bruto", projectedGross), in("creditos", projectedCredits))

	return TaxBreakdown{
		GrossTax:   projectedGross,
		Credits:    projectedCredits,
		NetTax:     projectedNet,
		Components: components,
		Trace:      t.steps,
	}
}

func finalizeResult(year int, regime string, current, projected TaxBreakdown) SimulationResult {
	delta := projected.NetTax.Sub(current.NetTax).Round(2)
	deltaPct := decimal.Zero
	if current.NetTax.IsPositive() {
		deltaPct = delta.Div(current.NetTax).Mul(decimal.NewFromInt(100)).Round(2)
	}
	return SimulationResult{
		Year:      year,
		Regime:    regime,
		Current:   current,
		Projected: projected,
		Delta:     delta,
		DeltaPct:  deltaPct,
	}
}

func isMEISimulation(regime, _ string) bool {
	return strings.EqualFold(strings.TrimSpace(regime), CompanyRegimeMEI)
}
