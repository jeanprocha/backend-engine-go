package lawcorpus

import (
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

func TestBuild_EmptyCorpus_SlicesAreNeverNil(t *testing.T) {
	view := Build(nil, "")

	if view.Documents == nil {
		t.Error("Documents não deveria ser nil no corpus vazio — serializa \"null\" em JSON")
	}
	if len(view.Documents) != 0 {
		t.Errorf("esperava 0 documentos, obteve %d", len(view.Documents))
	}
	if view.Changelog == nil {
		t.Error("Changelog não deveria ser nil no corpus vazio — serializa \"null\" em JSON")
	}
	if len(view.Changelog) != 0 {
		t.Errorf("esperava 0 entradas de changelog, obteve %d", len(view.Changelog))
	}
	if view.CurrentDocumentID != "" {
		t.Errorf("esperava current_document_id vazio, obteve %q", view.CurrentDocumentID)
	}
}

func TestBuild_KnownSource_UsesLabelURLFromCatalog(t *testing.T) {
	rows := []ingestion.CorpusDocumentRow{
		{Source: "LC 68/2024", LeiPDFVersion: "2024-07-22-doc-plp-68", StructureVersion: "1", ChunkPrefix: "lc68_", Chunks: 494},
	}
	view := Build(rows, "")

	if len(view.Documents) != 1 {
		t.Fatalf("esperava 1 documento, obteve %d", len(view.Documents))
	}
	doc := view.Documents[0]
	if doc.ID != "lc68-2024" {
		t.Errorf("ID = %q, esperava lc68-2024", doc.ID)
	}
	if doc.Label != "LC 68/2024" {
		t.Errorf("Label = %q, esperava LC 68/2024", doc.Label)
	}
	if doc.SourceURL == "" {
		t.Error("SourceURL não deveria ser vazio para fonte conhecida")
	}
	if doc.Version != "1" {
		t.Errorf("Version = %q, esperava 1 (structure_version)", doc.Version)
	}
	if doc.PublishedAt != "2024-07-22" {
		t.Errorf("PublishedAt = %q, esperava 2024-07-22 (derivado de lei_pdf_version)", doc.PublishedAt)
	}
	if doc.ChunkPrefix != "lc68_" {
		t.Errorf("ChunkPrefix = %q, esperava lc68_", doc.ChunkPrefix)
	}
}

func TestBuild_UnknownSource_LabelIsRawSourceAndURLEmpty(t *testing.T) {
	rows := []ingestion.CorpusDocumentRow{
		{Source: "Resolução CGIBS 99/2099", LeiPDFVersion: "", StructureVersion: "1", ChunkPrefix: "cgibs99_", Chunks: 3},
	}
	view := Build(rows, "")

	if len(view.Documents) != 1 {
		t.Fatalf("esperava 1 documento, obteve %d", len(view.Documents))
	}
	doc := view.Documents[0]
	if doc.Label != "Resolução CGIBS 99/2099" {
		t.Errorf("Label = %q, esperava o próprio source (fonte desconhecida)", doc.Label)
	}
	if doc.SourceURL != "" {
		t.Errorf("SourceURL = %q, esperava vazio (fonte desconhecida, sem URL a inventar)", doc.SourceURL)
	}
	if doc.ID != "Resolução CGIBS 99/2099" {
		t.Errorf("ID = %q, esperava o próprio source como fallback de ID", doc.ID)
	}
}

func TestBuild_LeiPDFVersionSemDataNoPrefixo_FallbackParaCatalogoOuVazio(t *testing.T) {
	rows := []ingestion.CorpusDocumentRow{
		{Source: "LC 68/2024", LeiPDFVersion: "versao-sem-data", StructureVersion: "1", ChunkPrefix: "lc68_", Chunks: 10},
	}
	view := Build(rows, "")
	// Fonte conhecida: cai no PublishedAt do catálogo (2024-07-22), não fica vazio.
	if view.Documents[0].PublishedAt != "2024-07-22" {
		t.Errorf("PublishedAt = %q, esperava fallback do catálogo (2024-07-22)", view.Documents[0].PublishedAt)
	}

	rowsUnknown := []ingestion.CorpusDocumentRow{
		{Source: "Documento Desconhecido", LeiPDFVersion: "versao-sem-data", StructureVersion: "1", ChunkPrefix: "x_", Chunks: 10},
	}
	viewUnknown := Build(rowsUnknown, "")
	if viewUnknown.Documents[0].PublishedAt != "" {
		t.Errorf("PublishedAt = %q, esperava vazio (fonte desconhecida sem data derivável)", viewUnknown.Documents[0].PublishedAt)
	}
}

func TestBuild_CurrentDocument_PicksMostChunksByDefault(t *testing.T) {
	rows := []ingestion.CorpusDocumentRow{
		{Source: "LC 68/2024", LeiPDFVersion: "2024-07-22-doc-plp-68", StructureVersion: "1", ChunkPrefix: "lc68_", Chunks: 494},
		{Source: "LC 214/2025", LeiPDFVersion: "2025-01-16-lc-214", StructureVersion: "1", ChunkPrefix: "lc214_", Chunks: 700},
	}
	view := Build(rows, "")
	if view.CurrentDocumentID != "lc214-2025" {
		t.Errorf("current_document_id = %q, esperava lc214-2025 (mais chunks)", view.CurrentDocumentID)
	}
}

func TestBuild_CurrentDocument_OverrideVenceContagem(t *testing.T) {
	rows := []ingestion.CorpusDocumentRow{
		{Source: "LC 68/2024", LeiPDFVersion: "2024-07-22-doc-plp-68", StructureVersion: "1", ChunkPrefix: "lc68_", Chunks: 700},
		{Source: "LC 214/2025", LeiPDFVersion: "2025-01-16-lc-214", StructureVersion: "1", ChunkPrefix: "lc214_", Chunks: 5},
	}
	view := Build(rows, "LC 214/2025")
	if view.CurrentDocumentID != "lc214-2025" {
		t.Errorf("current_document_id = %q, esperava lc214-2025 (override vence mesmo com menos chunks)", view.CurrentDocumentID)
	}
}

func TestBuild_ChangelogEntry_IsFactualNotInvented(t *testing.T) {
	rows := []ingestion.CorpusDocumentRow{
		{Source: "LC 68/2024", LeiPDFVersion: "2024-07-22-doc-plp-68", StructureVersion: "1", ChunkPrefix: "lc68_", Chunks: 494},
	}
	view := Build(rows, "")
	if len(view.Changelog) != 1 {
		t.Fatalf("esperava 1 entrada de changelog, obteve %d", len(view.Changelog))
	}
	entry := view.Changelog[0]
	if entry.Type != "rule" {
		t.Errorf("Type = %q, esperava rule", entry.Type)
	}
	want := "Corpus normativo indexado no TribIA: 494 trechos, data-base 22/07/2024."
	if entry.Desc != want {
		t.Errorf("Desc = %q, esperava %q", entry.Desc, want)
	}
}

func TestBuild_SkipsRowsWithEmptySource(t *testing.T) {
	rows := []ingestion.CorpusDocumentRow{
		{Source: "", LeiPDFVersion: "", StructureVersion: "", ChunkPrefix: "", Chunks: 2},
		{Source: "LC 68/2024", LeiPDFVersion: "2024-07-22-doc-plp-68", StructureVersion: "1", ChunkPrefix: "lc68_", Chunks: 494},
	}
	view := Build(rows, "")
	if len(view.Documents) != 1 {
		t.Errorf("esperava 1 documento (linha sem source ignorada), obteve %d", len(view.Documents))
	}
}
