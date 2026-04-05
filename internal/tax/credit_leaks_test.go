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
