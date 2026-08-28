package http

import (
	"encoding/json"
	"testing"
)

func TestClassificationHistorySnapshotJSON_preservesEvidence(t *testing.T) {
	snap := ClassificationHistorySnapshot{
		SnapshotVersion: 1,
		ExpenseClassifications: []BatchClassificationItem{
			{
				Description: "Material",
				IsEligible:  true,
				Confidence:  0.9,
				Evidence: []EvidenceArticleResponse{
					{ArticleID: "art1", Content: "texto", Similarity: 0.88},
				},
			},
		},
	}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var out ClassificationHistorySnapshot
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.ExpenseClassifications) != 1 {
		t.Fatalf("expense_classifications: got %d", len(out.ExpenseClassifications))
	}
	if len(out.ExpenseClassifications[0].Evidence) != 1 {
		t.Fatalf("evidence lost")
	}
	if out.ExpenseClassifications[0].Evidence[0].ArticleID != "art1" {
		t.Fatalf("article id: %q", out.ExpenseClassifications[0].Evidence[0].ArticleID)
	}
}

// TestClassificationHistorySnapshotJSON_preservesConsultantOverride prova o
// achado 9 (Etapa C/PR4): antes de ConsultantOverride existir em
// BatchClassificationItem, este round-trip (o mesmo caminho que
// saveSimulationRecordHandler/getSimulationRecordHandler fazem via JSONB)
// descartava o override em silêncio. É o teste que teria falhado antes do fix.
func TestClassificationHistorySnapshotJSON_preservesConsultantOverride(t *testing.T) {
	snap := ClassificationHistorySnapshot{
		SnapshotVersion: 1,
		ExpenseClassifications: []BatchClassificationItem{
			{
				Description: "Licença de software ERP",
				IsEligible:  true,
				RegimeType:  "padrao",
				ConsultantOverride: &ConsultantOverrideResponse{
					IsEligible:    false,
					RegimeType:    "padrao",
					Justification: "Não há nexo documental com a receita tributável.",
					OverriddenAt:  "2026-08-28T14:30:00Z",
				},
			},
			{
				Description: "Consultoria tributária",
				IsEligible:  true,
				RegimeType:  "padrao",
				// Sem override — deve permanecer nil, não um struct zerado.
			},
		},
	}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var out ClassificationHistorySnapshot
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.ExpenseClassifications) != 2 {
		t.Fatalf("expense_classifications: got %d", len(out.ExpenseClassifications))
	}

	overridden := out.ExpenseClassifications[0].ConsultantOverride
	if overridden == nil {
		t.Fatal("consultant_override foi descartado no round-trip")
	}
	if overridden.IsEligible != false || overridden.RegimeType != "padrao" {
		t.Fatalf("consultant_override decisão: got %+v", overridden)
	}
	if overridden.Justification != "Não há nexo documental com a receita tributável." {
		t.Fatalf("consultant_override justification: got %q", overridden.Justification)
	}
	if overridden.OverriddenAt != "2026-08-28T14:30:00Z" {
		t.Fatalf("consultant_override overridden_at: got %q", overridden.OverriddenAt)
	}

	if out.ExpenseClassifications[1].ConsultantOverride != nil {
		t.Fatalf("item sem override não deveria ganhar consultant_override no round-trip: got %+v",
			out.ExpenseClassifications[1].ConsultantOverride)
	}

	// A IA nunca é reescrita: is_eligible/regime_type na raiz continuam a
	// sugestão original mesmo quando há override — é o consumidor
	// (getEffectiveExpenseSimulationFields no frontend) quem escolhe.
	if !out.ExpenseClassifications[0].IsEligible {
		t.Fatal("is_eligible da raiz (sugestão da IA) não deveria mudar por causa do override")
	}
}
