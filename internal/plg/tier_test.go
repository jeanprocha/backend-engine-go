package plg

import "testing"

func TestMostRestrictivePlan(t *testing.T) {
	tests := []struct {
		a, b Plan
		want Plan
	}{
		{PlanFree, PlanPro, PlanFree},
		{PlanPro, PlanFree, PlanFree},
		{PlanPro, PlanPremium, PlanPro},
		{PlanPremium, PlanPro, PlanPro},
		{PlanFree, PlanPremium, PlanFree},
		{PlanPremium, PlanFree, PlanFree},
		{PlanPro, PlanPro, PlanPro},
	}
	for _, tc := range tests {
		if g := MostRestrictivePlan(tc.a, tc.b); g != tc.want {
			t.Fatalf("MostRestrictivePlan(%q,%q)=%q want %q", tc.a, tc.b, g, tc.want)
		}
	}
}
