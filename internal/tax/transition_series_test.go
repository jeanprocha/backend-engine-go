package tax

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

func TestTransitionSeries_EightYears(t *testing.T) {
	ctx := context.Background()
	eng := NewCalculator()
	base := SimulationInput{
		Year: 2028,
		Services: []Service{
			{
				ID:          "1",
				Description: "S",
				Amount:      decimal.NewFromInt(10_000),
				ISSRate:     decimal.RequireFromString("0.05"),
				RegimeType:  RegimePadrao,
			},
		},
		Expenses: nil,
	}
	series, err := TransitionSeries(ctx, eng, base)
	if err != nil {
		t.Fatalf("TransitionSeries: %v", err)
	}
	if len(series) != 8 {
		t.Fatalf("len: got %d want 8", len(series))
	}
	for i, r := range series {
		wantYear := 2026 + i
		if r.Year != wantYear {
			t.Fatalf("series[%d].Year: got %d want %d", i, r.Year, wantYear)
		}
	}
}

func TestTransitionSeries_ProjectedRateNonDecreasing(t *testing.T) {
	ctx := context.Background()
	eng := NewCalculator()
	base := SimulationInput{
		Services: []Service{
			{
				ID:          "1",
				Description: "S",
				Amount:      decimal.NewFromInt(100_000),
				ISSRate:     decimal.RequireFromString("0.05"),
				RegimeType:  RegimePadrao,
			},
		},
	}
	series, err := TransitionSeries(ctx, eng, base)
	if err != nil {
		t.Fatalf("TransitionSeries: %v", err)
	}
	var prev decimal.Decimal
	for i, r := range series {
		rules := RulesForYear(r.Year)
		combined := rules.CombinedProjectedRate()
		if i > 0 && combined.Cmp(prev) < 0 {
			t.Fatalf("year %d: CombinedProjectedRate %s < prev %s", r.Year, combined, prev)
		}
		prev = combined
	}
}
