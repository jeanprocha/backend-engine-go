package classifier

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFilterSnippetsInContent(t *testing.T) {
	content := "O IBS incide sobre serviços de educação e saúde."
	got := filterSnippetsInContent(content, []string{"serviços de educação", "não existe", "IBS"})
	want := []string{"serviços de educação", "IBS"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestExtractMatchInContent_CaseFold(t *testing.T) {
	content := "Crédito presumido sobre bens."
	if extractMatchInContent(content, "crédito presumido") != "Crédito presumido" {
		t.Fatal(extractMatchInContent(content, "crédito presumido"))
	}
}

func TestApplyEvidenceHighlights(t *testing.T) {
	llm := &classificationLLMResponse{
		EvidenceHighlights: []EvidenceHighlightEntry{
			{EvidenceIndex: 1, Snippets: []string{"alpha"}, SnippetsTentative: []string{"beta"}},
		},
	}
	ev := []EvidenceArticle{
		{Content: "x alpha y"},
		{Content: "beta gamma"},
	}
	applyEvidenceHighlights(llm, ev)
	if ev[0].RelevantSnippets[0] != "alpha" {
		t.Fatalf("snippet: %#v", ev[0].RelevantSnippets)
	}
	if len(ev[1].RelevantSnippets) != 0 {
		t.Fatalf("expected no index-1 snippets for first evidence only")
	}
}

func TestClipSnippetToMaxRunes(t *testing.T) {
	if got := clipSnippetToMaxRunes("ab", 10); got != "ab" {
		t.Fatalf("short: %q", got)
	}
	long := strings.Repeat("a", 200) + "END"
	clip := clipSnippetToMaxRunes(long, 180)
	if utf8.RuneCountInString(clip) != 180 {
		t.Fatalf("len %d", utf8.RuneCountInString(clip))
	}
	content := long + " tail"
	if filterSnippetsInContent(content, []string{long})[0] != clip {
		t.Fatalf("expected truncated match in content")
	}
}
