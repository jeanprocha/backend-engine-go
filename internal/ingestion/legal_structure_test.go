package ingestion

import (
	"strings"
	"testing"
)

func TestAnalyzeLegalPath_SingleParagrafo(t *testing.T) {
	const body = `Texto do caput inicial.
§ 1º Primeiro parágrafo com regra.
I - inciso um
II - inciso dois`
	p := AnalyzeLegalPath("Art. 4º", body)
	if p.Paragraph != "§ 1º" {
		t.Fatalf("paragraph: got %q want § 1º", p.Paragraph)
	}
	if p.Inciso != "I" {
		t.Fatalf("inciso: got %q want I", p.Inciso)
	}
	if p.SpanNote != "" {
		t.Fatalf("span_note: got %q", p.SpanNote)
	}
}

func TestAnalyzeLegalPath_MultiplosParagrafos(t *testing.T) {
	body := `§ 1º Um.
§ 2º Dois.`
	p := AnalyzeLegalPath("Art. 10º", body)
	if p.SpanNote == "" {
		t.Fatal("expected span_note for multiple §")
	}
	if p.Paragraph != "" {
		t.Fatalf("paragraph should be empty, got %q", p.Paragraph)
	}
}

func TestAnalyzeLegalPath_CaputEParagrafos(t *testing.T) {
	body := `I - inciso caput
II - outro
§ 1º parágrafo`
	p := AnalyzeLegalPath("Art. 5º", body)
	if p.SpanNote == "" {
		t.Fatal("expected caput+parágrafos note")
	}
}

func TestFormatLegalCitation(t *testing.T) {
	meta := map[string]string{
		MetaArticleLabel: "Art. 28º",
		MetaParagraph:    "§ 3º",
		MetaInciso:       "I",
		MetaAlinea:       "a",
	}
	s := FormatLegalCitation(meta)
	if !strings.Contains(s, "Art. 28º") || !strings.Contains(s, "§ 3º") || !strings.Contains(s, "inciso I") {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestFormatLegalCitation_SpanNote(t *testing.T) {
	meta := map[string]string{
		MetaArticleLabel: "Art. 1º",
		MetaSpanNote:     "trecho abrange múltiplos parágrafos",
	}
	s := FormatLegalCitation(meta)
	if !strings.Contains(s, "múltiplos") {
		t.Fatalf("expected span note in string: %q", s)
	}
}
