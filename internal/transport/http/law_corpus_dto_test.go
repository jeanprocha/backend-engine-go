package http

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestLawCorpusResponseJSON_EmptySlicesSerializeAsArrays cobre o bug mais
// provável de GET /law/corpus: um slice Go nil serializa "null" em JSON, e
// o frontend faz corpus.documents.find(...)/data.updates.map(...) sem
// checar null — um corpus vazio (nenhum chunk ingerido ainda, ou banco
// indisponível na leitura) derrubaria a UI em vez de cair no fallback.
func TestLawCorpusResponseJSON_EmptySlicesSerializeAsArrays(t *testing.T) {
	resp := LawCorpusResponse{
		Documents:         []LawCorpusDocumentResponse{},
		CurrentDocumentID: "",
		Changelog:         []LawCorpusChangelogEntryResponse{},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if !strings.Contains(s, `"documents":[]`) {
		t.Errorf("documents deveria serializar como [], obteve: %s", s)
	}
	if !strings.Contains(s, `"changelog":[]`) {
		t.Errorf("changelog deveria serializar como [], obteve: %s", s)
	}
	if strings.Contains(s, "null") {
		t.Errorf("nenhum campo deveria serializar null: %s", s)
	}
}

func TestLawCorpusResponseJSON_RoundTrip(t *testing.T) {
	resp := LawCorpusResponse{
		Documents: []LawCorpusDocumentResponse{
			{ID: "lc68-2024", Label: "LC 68/2024", Version: "1", PublishedAt: "2024-07-22", SourceURL: "https://example.org", ChunkPrefix: "lc68_"},
		},
		CurrentDocumentID: "lc68-2024",
		Changelog: []LawCorpusChangelogEntryResponse{
			{Type: "rule", Label: "LC 68/2024", Desc: "Corpus normativo indexado no TribIA: 494 trechos, data-base 22/07/2024."},
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var out LawCorpusResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Documents) != 1 || out.Documents[0].ID != "lc68-2024" {
		t.Errorf("round-trip de documents falhou: %+v", out.Documents)
	}
	if out.CurrentDocumentID != "lc68-2024" {
		t.Errorf("current_document_id = %q", out.CurrentDocumentID)
	}
	if len(out.Changelog) != 1 || out.Changelog[0].Type != "rule" {
		t.Errorf("round-trip de changelog falhou: %+v", out.Changelog)
	}
}
