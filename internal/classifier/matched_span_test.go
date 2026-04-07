package classifier

import "testing"

func TestNormalizeMatchedSpan(t *testing.T) {
	t.Parallel()
	ctx := "Empresa SaaS B2B"
	// "SaaS" starts at rune index after "Empresa " = 8? Let's count: E m p r e s a space = 8 runes, S a a S = positions 8-11, end 12
	sp := NormalizeMatchedSpan(ctx, 8, 12)
	if sp == nil {
		t.Fatal("expected span")
	}
	if got := string([]rune(ctx)[sp.Start:sp.End]); got != "SaaS" {
		t.Fatalf("substring: %q", got)
	}
	if NormalizeMatchedSpan("", 0, 1) != nil {
		t.Fatal("empty context")
	}
	if NormalizeMatchedSpan(ctx, -1, 3) != nil {
		t.Fatal("negative start")
	}
	if NormalizeMatchedSpan(ctx, 0, 0) != nil {
		t.Fatal("end <= start")
	}
	// clamp end
	sp2 := NormalizeMatchedSpan(ctx, 0, 999)
	if sp2 == nil || sp2.End != len([]rune(ctx)) {
		t.Fatalf("clamp end: %+v", sp2)
	}
}
