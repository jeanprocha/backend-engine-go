package tax

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestISSMunicipalTransitionFactor(t *testing.T) {
	cases := []struct {
		year int
		want string
	}{
		{2026, "1"},
		{2028, "1"},
		{2029, "0.8"},
		{2030, "0.6"},
		{2031, "0.4"},
		{2032, "0.2"},
		{2033, "0"},
	}
	for _, tc := range cases {
		r := RulesForYear(tc.year)
		got := r.ISSMunicipalTransitionFactor()
		want := decimal.RequireFromString(tc.want)
		if !got.Equal(want) {
			t.Fatalf("year %d: got %s want %s", tc.year, got, want)
		}
	}
}

func TestRulesForYear_2026_TestPhaseRates(t *testing.T) {
	r := RulesForYear(2026)
	if !r.CBSRate.Equal(decimal.RequireFromString("0.009")) {
		t.Fatalf("CBS 2026: %s", r.CBSRate)
	}
	if !r.IBSRate.Equal(decimal.RequireFromString("0.001")) {
		t.Fatalf("IBS 2026: %s", r.IBSRate)
	}
	if !r.PISCOFINSFactor.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("PIS/COFINS factor 2026: %s", r.PISCOFINSFactor)
	}
}

func TestRulesForYear_2033_FullProjected(t *testing.T) {
	r := RulesForYear(2033)
	if !r.PISCOFINSFactor.IsZero() {
		t.Fatalf("PIS/COFINS extinto 2033: %s", r.PISCOFINSFactor)
	}
	if !r.CBSRate.Equal(decimal.RequireFromString("0.099")) {
		t.Fatalf("CBS 2033: %s", r.CBSRate)
	}
	if !r.IBSRate.Equal(decimal.RequireFromString("0.166")) {
		t.Fatalf("IBS 2033: %s", r.IBSRate)
	}
}
