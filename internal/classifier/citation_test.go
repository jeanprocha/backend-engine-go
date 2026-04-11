package classifier

import (
	"strings"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

func TestApplyDeterministicCitation_ValidIndex(t *testing.T) {
	ev := []EvidenceArticle{
		{ArticleID: "anchor", Metadata: map[string]string{"article_id": "Art. 26º"}},
		{
			ArticleID: "lc68_0100_art_100",
			Metadata: map[string]string{
				ingestion.MetaArticleLabel: "Art. 100º",
				ingestion.MetaParagraph:    "§ 2º",
				"article_id":               "Art. 100º",
			},
		},
	}
	one := 2
	llm := &classificationLLMResponse{RiskLevel: "baixo", PrimaryEvidenceIndex: &one}
	base, risk := applyDeterministicCitation(llm, ev)
	if risk != "baixo" {
		t.Fatalf("risk: %s", risk)
	}
	if base == "" || base == "anchor" {
		t.Fatalf("legal base: %q", base)
	}
	if !strings.Contains(base, "Art. 100º") || !strings.Contains(base, "§ 2º") {
		t.Fatalf("unexpected base: %q", base)
	}
}

func TestApplyDeterministicCitation_InvalidIndex(t *testing.T) {
	ev := []EvidenceArticle{{ArticleID: "a", Metadata: map[string]string{"article_id": "Art. 1º"}}}
	nine := 9
	llm := &classificationLLMResponse{RiskLevel: "baixo", PrimaryEvidenceIndex: &nine}
	base, risk := applyDeterministicCitation(llm, ev)
	if base != "" {
		t.Fatalf("expected empty base, got %q", base)
	}
	if risk != "alto" {
		t.Fatalf("expected alto risk, got %q", risk)
	}
}

func TestPickDefaultEvidenceIndex_SkipsAnchors(t *testing.T) {
	ev := []EvidenceArticle{
		{ArticleID: anchorArticleIDs[0]},
		{ArticleID: "semantic_chunk"},
	}
	if i := pickDefaultEvidenceIndex(ev); i != 1 {
		t.Fatalf("got %d", i)
	}
}

