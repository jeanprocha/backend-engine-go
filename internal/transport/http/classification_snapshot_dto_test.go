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
