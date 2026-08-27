package classifier

import (
	"reflect"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

func TestParseAnchorArticleIDs(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"vazio", "", nil},
		{"só espaços", "   ", nil},
		{"csv simples", "a,b,c", []string{"a", "b", "c"}},
		{"com espaços e vírgulas duplicadas", " a , b ,, c ", []string{"a", "b", "c"}},
		{"um só id", "lc214_0001_art_1", []string{"lc214_0001_art_1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAnchorArticleIDs(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseAnchorArticleIDs(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestMissingAnchorIDs(t *testing.T) {
	requested := []string{"a", "b", "c"}
	found := []ingestion.SearchResult{{ArticleID: "a"}, {ArticleID: "c"}}

	got := missingAnchorIDs(requested, found)
	want := []string{"b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMissingAnchorIDs_AllFound(t *testing.T) {
	requested := []string{"a", "b"}
	found := []ingestion.SearchResult{{ArticleID: "a"}, {ArticleID: "b"}}

	got := missingAnchorIDs(requested, found)
	if len(got) != 0 {
		t.Errorf("esperava nenhum ID faltando, obteve %v", got)
	}
}

func TestMissingAnchorIDs_NoneFound(t *testing.T) {
	requested := []string{"a", "b"}
	got := missingAnchorIDs(requested, nil)
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDefaultLawLabel_MatchesIngestionDefaultDocumentProfile(t *testing.T) {
	// defaultLawLabel() é um passthrough deliberado para
	// ingestion.DefaultDocumentProfile().SourceLabel (classifier já importa
	// ingestion) — travando isso aqui, um refactor futuro que quebre o
	// passthrough (ex.: hardcodar de novo um literal) falha alto.
	if got, want := defaultLawLabel(), ingestion.DefaultDocumentProfile().SourceLabel; got != want {
		t.Errorf("defaultLawLabel() = %q, esperava %q (ingestion.DefaultDocumentProfile().SourceLabel)", got, want)
	}
}
