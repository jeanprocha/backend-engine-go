package ingestion

import (
	"path/filepath"
	"testing"
)

func TestApplyLeiArticlePageMap(t *testing.T) {
	m := &LeiArticlePageMap{
		LeiVersion: ExpectedLeiPDFMapVersions[0],
		Convention: PdfCoordConventionYNormalized01,
		Articles: map[string]ArticlePageEntry{
			"Art. 2º": {Page: 5, PdfCoordY: "0.25"},
		},
	}
	chunks := []ArticleChunk{
		{
			Metadata: map[string]string{
				"article_id": "Art. 2º",
				"source":     "LC 68/2024",
			},
		},
		{
			Metadata: map[string]string{
				"article_id": "Art. 999º",
				"source":     "LC 68/2024",
			},
		},
	}
	ApplyLeiArticlePageMap(chunks, m)
	if chunks[0].Metadata[MetaPdfPage] != "5" {
		t.Fatalf("pdf_page: %q", chunks[0].Metadata[MetaPdfPage])
	}
	if chunks[0].Metadata[MetaPdfCoordY] != "0.25" {
		t.Fatalf("pdf_coord_y: %q", chunks[0].Metadata[MetaPdfCoordY])
	}
	if chunks[1].Metadata[MetaPdfPage] != "" {
		t.Fatal("artigo fora do mapa não deve receber pdf_page")
	}
}

func TestLoadLeiArticlePageMap_fixture(t *testing.T) {
	// go test executa com cwd = pasta do pacote (internal/ingestion).
	path := filepath.Join("..", "..", "docs", "legislacao", "lc68_article_page_map.json")
	m, err := LoadLeiArticlePageMap(path)
	if err != nil {
		t.Skip("fixture ausente:", err)
	}
	if !isExpectedLeiPDFMapVersion(m.LeiVersion) {
		t.Fatalf("lei_version: %q", m.LeiVersion)
	}
	if len(m.Articles) < 1 {
		t.Fatal("mapa vazio")
	}
}

func TestIsExpectedLeiPDFMapVersion(t *testing.T) {
	if !isExpectedLeiPDFMapVersion(ExpectedLeiPDFMapVersions[0]) {
		t.Error("a primeira versão da lista deveria ser aceita")
	}
	if isExpectedLeiPDFMapVersion("versao-inexistente") {
		t.Error("versão fora da lista não deveria ser aceita")
	}
}
