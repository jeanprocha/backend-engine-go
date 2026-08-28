package tax

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestNormalizeExpenseRegimeType_emptyIsPadrao(t *testing.T) {
	if got := NormalizeExpenseRegimeType(""); got != RegimePadrao {
		t.Fatalf("empty: got %q want %q", got, RegimePadrao)
	}
	if got := NormalizeExpenseRegimeType("   "); got != RegimePadrao {
		t.Fatalf("spaces: got %q want %q", got, RegimePadrao)
	}
	if got := NormalizeExpenseRegimeType("regular"); got != RegimePadrao {
		t.Fatalf("regular: got %q want %q", got, RegimePadrao)
	}
}

func TestCreditLeaksSupported_excludesProfiles(t *testing.T) {
	if CreditLeaksSupported(CompanyRegimeMEI) {
		t.Fatal("MEI should not support credit leaks")
	}
	if CreditLeaksSupported(CompanyRegimeSimplesPuro) {
		t.Fatal("simples_puro should not support credit leaks")
	}
	// entidade imune slug
	if CreditLeaksSupported("entidade_imune") {
		t.Fatal("entidade_imune should not support credit leaks")
	}
	if !CreditLeaksSupported("") {
		t.Fatal("empty company regime should support leaks")
	}
	if !CreditLeaksSupported("regular") {
		t.Fatal("regular company regime should support leaks")
	}
}

func TestBuildCreditLeaks_ineligibleEmptyRegimeUsesPadraoRate(t *testing.T) {
	year := 2026
	ex := []Expense{
		{
			ID:          "e1",
			Description: "Teste AWS",
			Amount:      decimal.NewFromInt(1000),
			IsEligible:  false,
			RegimeType:  "",
		},
	}
	leaks := BuildCreditLeaks(year, ex)
	if len(leaks) != 1 {
		t.Fatalf("len(leaks) = %d, want 1", len(leaks))
	}
	rules := RulesForYear(year)
	want := decimal.NewFromInt(1000).Mul(rules.EffectiveProjectedRate(RegimePadrao)).Round(2)
	if !leaks[0].LostCredit.Equal(want) {
		t.Fatalf("LostCredit = %s, want %s", leaks[0].LostCredit, want)
	}
	if leaks[0].RegimeType != RegimePadrao {
		t.Fatalf("RegimeType = %q, want %q", leaks[0].RegimeType, RegimePadrao)
	}
}

func TestBuildCreditLeaks_skipsEligible(t *testing.T) {
	ex := []Expense{
		{ID: "a", Amount: decimal.NewFromInt(500), IsEligible: true, RegimeType: RegimePadrao},
	}
	if leaks := BuildCreditLeaks(2026, ex); len(leaks) != 0 {
		t.Fatalf("expected no leaks, got %d", len(leaks))
	}
}

// TestBuildCreditLeaks_annualValuesCoverTransition prova que AnnualValues
// cobre exatamente 2026-2033, na ordem, com a mesma alíquota por ano que o
// resto do motor usa (RulesForYear(y).EffectiveProjectedRate) — e que o ano
// simulado bate exatamente com o LostCredit "avulso" já existente.
func TestBuildCreditLeaks_annualValuesCoverTransition(t *testing.T) {
	ex := []Expense{{ID: "e1", Description: "Consultoria", Amount: decimal.NewFromInt(1000), IsEligible: false, RegimeType: RegimePadrao}}
	leaks := BuildCreditLeaks(2026, ex)
	if len(leaks) != 1 {
		t.Fatalf("len(leaks) = %d, want 1", len(leaks))
	}
	annual := leaks[0].AnnualValues
	if len(annual) != 8 {
		t.Fatalf("len(AnnualValues) = %d, want 8 (2026..2033)", len(annual))
	}
	for i, y := range []int{2026, 2027, 2028, 2029, 2030, 2031, 2032, 2033} {
		if annual[i].Year != y {
			t.Fatalf("AnnualValues[%d].Year = %d, want %d", i, annual[i].Year, y)
		}
		want := decimal.NewFromInt(1000).Mul(RulesForYear(y).EffectiveProjectedRate(RegimePadrao)).Round(2)
		if !annual[i].LostCredit.Equal(want) {
			t.Fatalf("AnnualValues[%d] (%d) = %s, want %s", i, y, annual[i].LostCredit, want)
		}
	}
	// O ano simulado (2026) tem que bater com o campo avulso já existente —
	// não é um segundo número, é o mesmo.
	if !annual[0].LostCredit.Equal(leaks[0].LostCredit) {
		t.Fatalf("AnnualValues[0] (%s) diverge de LostCredit (%s) para o ano simulado", annual[0].LostCredit, leaks[0].LostCredit)
	}
	// Amount = R$1.000 em padrao soma R$980,00 na transição inteira (rampa
	// travada em rules_table_test.go) — número exato, não faixa.
	total := sumAnnualValues(annual)
	want := decimal.RequireFromString("980.00")
	if !total.Equal(want) {
		t.Fatalf("total anualizado = %s, want %s", total, want)
	}
}

// TestBuildCreditLeaks_effortRiskPriority prova as faixas determinísticas:
// Effort é regime-only; Priority é valor-only; Risk mistura os dois (o
// modificador de prof_liberal precisa sobrepor a faixa de valor, senão os
// dois campos seriam a mesma função disfarçada — ver achado da PR5 no
// roadmap: "esforço/risco viram julgamento arbitrário disfarçado de dado").
func TestBuildCreditLeaks_effortRiskPriority(t *testing.T) {
	cases := []struct {
		name         string
		amount       int64
		regime       string
		wantEffort   string
		wantRisk     string
		wantPriority string
	}{
		{"padrao baixo valor", 1000, RegimePadrao, "baixo", "baixo", "baixa"},
		{"padrao alto valor", 20000, RegimePadrao, "baixo", "alto", "alta"},
		{"diferenciado_60 medio valor", 10000, RegimeDiferenciado60, "medio", "medio", "media"},
		// prof_liberal: risco alto mesmo com valor baixo (premissa TribIA,
		// não prevista em lei) — mas prioridade continua só-valor (baixa).
		{"prof_liberal baixo valor mas risco alto por premissa", 1000, RegimeProfissionalLiberal, "alto", "alto", "baixa"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex := []Expense{{ID: "e1", Amount: decimal.NewFromInt(tc.amount), IsEligible: false, RegimeType: tc.regime}}
			leaks := BuildCreditLeaks(2026, ex)
			if len(leaks) != 1 {
				t.Fatalf("len(leaks) = %d, want 1", len(leaks))
			}
			l := leaks[0]
			if l.Effort != tc.wantEffort {
				t.Errorf("Effort = %q, want %q", l.Effort, tc.wantEffort)
			}
			if l.Risk != tc.wantRisk {
				t.Errorf("Risk = %q, want %q", l.Risk, tc.wantRisk)
			}
			if l.Priority != tc.wantPriority {
				t.Errorf("Priority = %q, want %q", l.Priority, tc.wantPriority)
			}
		})
	}
}

// TestBuildCreditLeaks_legalBasePassthrough prova que LegalBase só ecoa
// Expense.LegalBase (aparado) — nunca é inventada quando ausente.
func TestBuildCreditLeaks_legalBasePassthrough(t *testing.T) {
	ex := []Expense{
		{ID: "e1", Description: "Com citação", Amount: decimal.NewFromInt(1000), IsEligible: false, RegimeType: RegimePadrao, LegalBase: "  Art. 47, LC 214/2025  "},
		{ID: "e2", Description: "Sem citação", Amount: decimal.NewFromInt(1000), IsEligible: false, RegimeType: RegimePadrao, LegalBase: ""},
	}
	leaks := BuildCreditLeaks(2026, ex)
	if len(leaks) != 2 {
		t.Fatalf("len(leaks) = %d, want 2", len(leaks))
	}
	if leaks[0].LegalBase != "Art. 47, LC 214/2025" {
		t.Fatalf("LegalBase[0] = %q, want aparado", leaks[0].LegalBase)
	}
	if leaks[1].LegalBase != "" {
		t.Fatalf("LegalBase[1] = %q, want vazio (sem citação, não inventar)", leaks[1].LegalBase)
	}
}
