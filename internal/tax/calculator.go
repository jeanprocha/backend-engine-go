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

	// MEI: premissa mensal com DAS fixo (ilustrativo); não incide CBS/IBS nem PIS/COFINS/ISS da receita.
	// Ativa apenas com company_regime "mei".
	if isMEISimulation(input.CompanyRegime, input.CompanyContext) {
		fixed := MEIMonthlyDAS().Round(2)
		return SimulationResult{
			Year: input.Year,
			Current: TaxBreakdown{
				GrossTax: fixed,
				Credits:  decimal.Zero,
				NetTax:   fixed,
			},
			Projected: TaxBreakdown{
				GrossTax: fixed,
				Credits:  decimal.Zero,
				NetTax:   fixed,
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
		current := TaxBreakdown{
			GrossTax: currentGross,
			Credits:  decimal.Zero,
			NetTax:   currentGross,
		}
		var projected TaxBreakdown
		if strings.EqualFold(strings.TrimSpace(input.CompanyRegime), CompanyRegimeSimplesPuro) {
			pg := totalRevenue.Mul(SimplesPuroEffectiveIBSCBSRate()).Round(2)
			projected = TaxBreakdown{GrossTax: pg, Credits: decimal.Zero, NetTax: pg}
		} else {
			projected = computeProjectedCBSIBS(rules, input.Services, input.Expenses)
		}
		return finalizeResult(input.Year, current, projected), nil
	}

	// Perfil cesta básica / social: atual = regular; projetado força alíquota CBS+IBS zero em toda a receita
	// (EffectiveProjectedRate(RegimeReduzidoZero)); créditos por despesa — líquido projetado pode ser negativo.
	if IsAliquotaZeroProfile(input.CompanyRegime) {
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		projected := computeProjectedCBSIBSForcedOutputRegime(rules, input.Services, input.Expenses, RegimeReduzidoZero)
		return finalizeResult(input.Year, current, projected), nil
	}

	// Perfil exportadora (ilustrativo): imunidade integral CBS+IBS na saída; créditos nas compras — alíquota efetiva zero na receita (mesma conta que cesta básica na projeção).
	if IsExportadoraProfile(input.CompanyRegime) {
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		projected := computeProjectedCBSIBSForcedOutputRegime(rules, input.Services, input.Expenses, RegimeReduzidoZero)
		return finalizeResult(input.Year, current, projected), nil
	}

	// Perfil entidade imune (ilustrativo): saída CBS+IBS zero; sem créditos no projetado (consumidor final no modelo).
	if IsEntidadeImuneProfile(input.CompanyRegime) {
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		gross := projectedGrossCBSIBSForcedOutputRegime(rules, input.Services, RegimeReduzidoZero)
		projected := TaxBreakdown{
			GrossTax: gross,
			Credits:  decimal.Zero,
			NetTax:   gross,
		}
		return finalizeResult(input.Year, current, projected), nil
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
		effective := rules.CombinedProjectedRate().Mul(mult).Round(6)
		projected := computeProjectedImobiliario(rules, totalRevenue, input.Expenses, effective, input.ImobiliarioRedutorAjusteBRL)
		return finalizeResult(input.Year, current, projected), nil
	}

	// Perfil profissões regulamentadas (ilustrativo): atual = regular; projetado força 70% da alíquota padrão CBS+IBS (redução ilustrativa de 30%).
	if IsProfissionalLiberalProfile(input.CompanyRegime) {
		current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
		if err != nil {
			return SimulationResult{}, err
		}
		projected := computeProjectedCBSIBSForcedOutputRegime(rules, input.Services, input.Expenses, RegimeProfissionalLiberal)
		return finalizeResult(input.Year, current, projected), nil
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
		return finalizeResult(input.Year, current, projected), nil
	}

	current, err := computeCurrentRegularLegacy(rules, totalRevenue, input.Services, input.Expenses)
	if err != nil {
		return SimulationResult{}, err
	}
	projected := computeProjectedCBSIBS(rules, input.Services, input.Expenses)

	return finalizeResult(input.Year, current, projected), nil
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
	currentGross := totalRevenue.Mul(rules.CombinedCurrentRate())
	issLeg := rules.ISSMunicipalTransitionFactor()
	for _, svc := range services {
		effectiveISS := svc.ISSRate.Mul(issLeg)
		currentGross = currentGross.Add(svc.Amount.Mul(effectiveISS))
	}
	currentGross = currentGross.Round(2)

	currentCredits := decimal.Zero
	for _, exp := range expenses {
		if exp.IsEligible {
			if exp.Amount.IsNegative() {
				return TaxBreakdown{}, fmt.Errorf("calculator: despesa %q com valor negativo", exp.ID)
			}
			currentCredits = currentCredits.Add(exp.Amount.Mul(rules.CombinedCurrentRate()))
		}
	}
	currentCredits = currentCredits.Round(2)
	currentNet := currentGross.Sub(currentCredits).Round(2)
	return TaxBreakdown{
		GrossTax: currentGross,
		Credits:  currentCredits,
		NetTax:   currentNet,
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
func computeProjectedImobiliario(rules TaxRules, totalRevenue decimal.Decimal, expenses []Expense, effectiveRate, redutor decimal.Decimal) TaxBreakdown {
	taxable := totalRevenue.Sub(redutor)
	if taxable.IsNegative() {
		taxable = decimal.Zero
	}
	projectedGross := taxable.Mul(effectiveRate).Round(2)

	projectedCredits := decimal.Zero
	for _, exp := range expenses {
		if exp.IsEligible {
			creditRate := rules.EffectiveProjectedRate(exp.RegimeType)
			projectedCredits = projectedCredits.Add(exp.Amount.Mul(creditRate))
		}
	}
	projectedCredits = projectedCredits.Round(2)
	projectedNet := projectedGross.Sub(projectedCredits).Round(2)
	return TaxBreakdown{
		GrossTax: projectedGross,
		Credits:  projectedCredits,
		NetTax:   projectedNet,
	}
}

// projectedGrossCBSIBSForcedOutputRegime soma CBS+IBS sobre a receita de serviços. Se outputRegime não for vazio,
// toda a linha usa EffectiveProjectedRate(outputRegime); senão usa svc.RegimeType por serviço.
func projectedGrossCBSIBSForcedOutputRegime(rules TaxRules, services []Service, outputRegime string) decimal.Decimal {
	projectedGross := decimal.Zero
	for _, svc := range services {
		rt := svc.RegimeType
		if outputRegime != "" {
			rt = outputRegime
		}
		rate := rules.EffectiveProjectedRate(rt)
		projectedGross = projectedGross.Add(svc.Amount.Mul(rate))
	}
	return projectedGross.Round(2)
}

// computeProjectedCBSIBSForcedOutputRegime calcula CBS/IBS projetado. Se outputRegime não for vazio,
// toda a receita de serviços usa EffectiveProjectedRate(outputRegime); senão usa svc.RegimeType por linha.
// Créditos seguem sempre regime_type de cada despesa.
func computeProjectedCBSIBSForcedOutputRegime(rules TaxRules, services []Service, expenses []Expense, outputRegime string) TaxBreakdown {
	projectedGross := projectedGrossCBSIBSForcedOutputRegime(rules, services, outputRegime)

	projectedCredits := decimal.Zero
	for _, exp := range expenses {
		if exp.IsEligible {
			creditRate := rules.EffectiveProjectedRate(exp.RegimeType)
			projectedCredits = projectedCredits.Add(exp.Amount.Mul(creditRate))
		}
	}
	projectedCredits = projectedCredits.Round(2)
	projectedNet := projectedGross.Sub(projectedCredits).Round(2)
	return TaxBreakdown{
		GrossTax: projectedGross,
		Credits:  projectedCredits,
		NetTax:   projectedNet,
	}
}

func finalizeResult(year int, current, projected TaxBreakdown) SimulationResult {
	delta := projected.NetTax.Sub(current.NetTax).Round(2)
	deltaPct := decimal.Zero
	if current.NetTax.IsPositive() {
		deltaPct = delta.Div(current.NetTax).Mul(decimal.NewFromInt(100)).Round(2)
	}
	return SimulationResult{
		Year:      year,
		Current:   current,
		Projected: projected,
		Delta:     delta,
		DeltaPct:  deltaPct,
	}
}

func isMEISimulation(regime, _ string) bool {
	return strings.EqualFold(strings.TrimSpace(regime), CompanyRegimeMEI)
}
