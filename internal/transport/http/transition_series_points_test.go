package http

import (
	"context"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/tax"
	"github.com/shopspring/decimal"
)

func TestToTransitionSeriesPoints_IncludesFactorsAndBreakdown(t *testing.T) {
	eng := tax.NewCalculator()
	ctx := context.Background()
	base := tax.SimulationInput{
		Year: 2026,
		Services: []tax.Service{
			{ID: "1", Amount: decimal.NewFromInt(10_000), ISSRate: decimal.RequireFromString("0.05"), RegimeType: tax.RegimePadrao},
		},
		Expenses: []tax.Expense{{ID: "e1", Amount: decimal.NewFromInt(1000), IsEligible: true, RegimeType: tax.RegimePadrao}},
	}
	series, err := tax.TransitionSeries(ctx, eng, base)
	if err != nil {
		t.Fatal(err)
	}
	pts := toTransitionSeriesPoints(series)
	if len(pts) != 8 {
		t.Fatalf("len %d", len(pts))
	}
	p2026 := pts[0]
	if p2026.Year != 2026 {
		t.Fatalf("year %d", p2026.Year)
	}
	if p2026.Factors == nil {
		t.Fatal("expected factors")
	}
	if p2026.Factors.CbsRate != "0.009000" {
		t.Fatalf("cbs 2026: %q", p2026.Factors.CbsRate)
	}
	if p2026.Factors.IbsRate != "0.001000" {
		t.Fatalf("ibs 2026: %q", p2026.Factors.IbsRate)
	}
	if p2026.Current.GrossTax == "" || p2026.Projected.GrossTax == "" {
		t.Fatal("expected current/projected breakdown")
	}
	if p2026.Delta == "" || p2026.DeltaPct == "" {
		t.Fatal("expected delta")
	}
}

func TestEnrichTransitionSeriesLegacy_backfillsFactors(t *testing.T) {
	legacy := []TransitionSeriesPoint{
		{
			Year:        2026,
			OldTaxNet:   "10.00",
			NewTaxNet:   "20.00",
			TotalTaxNet: "30.00",
		},
	}
	out, enriched := enrichTransitionSeriesLegacy(legacy)
	if !enriched {
		t.Fatal("expected enrichment flag")
	}
	if len(out) != 1 || out[0].Factors == nil {
		t.Fatal("expected factors")
	}
	if out[0].Factors.CbsRate != "0.009000" {
		t.Fatalf("cbs: %q", out[0].Factors.CbsRate)
	}
	if out[0].Current.NetTax != "10.00" || out[0].Projected.NetTax != "20.00" {
		t.Fatalf("breakdown: %+v / %+v", out[0].Current, out[0].Projected)
	}
	if out[0].Delta != "10.00" {
		t.Fatalf("delta: %q", out[0].Delta)
	}
}

func TestEnrichTransitionSeriesLegacy_noChangeWhenComplete(t *testing.T) {
	complete := []TransitionSeriesPoint{
		{
			Year:        2026,
			OldTaxNet:   "10.00",
			NewTaxNet:   "20.00",
			TotalTaxNet: "30.00",
			Current:     TaxBreakdownResponse{GrossTax: "1", Credits: "0", NetTax: "10.00"},
			Projected:   TaxBreakdownResponse{GrossTax: "1", Credits: "0", NetTax: "20.00"},
			Delta:       "10.00",
			DeltaPct:    "100.00",
			Factors: &TransitionYearFactors{
				Year:            2026,
				PisCofinsFactor: "0.65",
				CbsRate:         "0.009000",
				IbsRate:         "0.001000",
			},
		},
	}
	_, enriched := enrichTransitionSeriesLegacy(complete)
	if enriched {
		t.Fatal("expected no enrichment when snapshot already complete")
	}
}
