package ingestion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArticles_LC68CleanSample(t *testing.T) {
	root := filepath.Join("..", "..", "docs", "lc68_2024_limpa.md")
	raw, err := os.ReadFile(root)
	if err != nil {
		t.Skip("arquivo de lei nao encontrado para teste local:", err)
	}

	chunks := NewParser(string(raw)).ParseArticles()
	if len(chunks) == 0 {
		t.Fatal("esperava pelo menos um artigo")
	}

	// Nenhum chunk deve ultrapassar o limite — este e o invariante critico.
	for _, c := range chunks {
		if len(c.Content) > maxChunkChars {
			t.Errorf("chunk %q excede o limite: %d chars", c.ID, len(c.Content))
		}
	}

	t.Logf("chunks gerados: %d (incluindo partes de artigos longos)", len(chunks))
	t.Logf("primeiro: id=%q", chunks[0].ID)
	t.Logf("ultimo:   id=%q", chunks[len(chunks)-1].ID)
}

func TestParseArticles_MarkdownHeader(t *testing.T) {
	text := "#### Art. 2º Texto do artigo segundo.\n\n#### Art. 3º Outro."
	chunks := NewParser(text).ParseArticles()
	if len(chunks) != 2 {
		t.Fatalf("esperava 2 chunks, obteve %d", len(chunks))
	}
}

func TestParseArticles_IgnoresInlineReference(t *testing.T) {
	text := "#### Art. 1º Conforme o art. 5º desta lei, aplica-se a regra.\n\n#### Art. 2º Outro artigo."
	chunks := NewParser(text).ParseArticles()
	if len(chunks) != 2 {
		t.Fatalf("esperava 2 chunks (ignorando referencia inline), obteve %d", len(chunks))
	}
}

func TestParseArticles_IDsAreUnique(t *testing.T) {
	text := "#### Art. 1º Texto.\n\n#### Art. 1º Texto repetido.\n\n#### Art. 2º Outro."
	chunks := NewParser(text).ParseArticles()

	seen := make(map[string]bool)
	for _, c := range chunks {
		if seen[c.ID] {
			t.Errorf("ID duplicado: %q", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestParseArticles_ContentDoesNotStartWithHash(t *testing.T) {
	text := "#### Art. 1º Conteudo do artigo.\n\n#### Art. 2º Outro."
	chunks := NewParser(text).ParseArticles()
	for _, c := range chunks {
		if strings.HasPrefix(c.Content, "#") {
			t.Errorf("chunk %q nao deve comecar com #", c.ID)
		}
	}
}

func TestSplitLongContent_SplitsIntoMultipleParts(t *testing.T) {
	// Gera um conteudo que excede maxChunkChars
	var sb strings.Builder
	para := strings.Repeat("palavra ", 200) + "fim do paragrafo."
	for sb.Len() < maxChunkChars*2 {
		sb.WriteString(para + "\n\n")
	}

	parts := splitLongContent("Art. 999º", sb.String())
	if len(parts) < 2 {
		t.Fatalf("esperava pelo menos 2 partes para conteudo longo, obteve %d", len(parts))
	}
	for i, p := range parts {
		if len(p) > maxChunkChars {
			t.Errorf("parte %d excede o limite: %d chars", i+1, len(p))
		}
		if p == "" {
			t.Errorf("parte %d esta vazia", i+1)
		}
	}
}

func TestSplitLongContent_ContextInContinuationParts(t *testing.T) {
	var sb strings.Builder
	para := strings.Repeat("x ", 400) + ".\n\n"
	for sb.Len() < maxChunkChars*2 {
		sb.WriteString(para)
	}

	parts := splitLongContent("Art. 100º", sb.String())
	if len(parts) < 2 {
		t.Skip("conteudo nao gerou multiplas partes neste cenario")
	}
	// Partes a partir da segunda devem conter "continuação" para que o RAG
	// saiba que e uma sequencia do mesmo artigo.
	if !strings.Contains(parts[1], "continuação") {
		t.Errorf("parte 2 deveria conter 'continuacao' para contexto de RAG")
	}
}

func TestParseArticles_LongArticleGeneratesParts(t *testing.T) {
	// Simula um artigo que excede maxChunkChars no MD gerado pelo cleaner.
	longContent := strings.Repeat("Inciso com texto longo. ", 400)
	text := "#### Art. 272º " + longContent + "\n\n#### Art. 273º Artigo normal."

	chunks := NewParser(text).ParseArticles()

	// Art. 272 deve ter virado multiplos chunks (partes).
	var partsOf272 int
	for _, c := range chunks {
		if strings.Contains(c.ID, "lc68_0001") {
			partsOf272++
		}
	}
	if partsOf272 < 2 {
		t.Errorf("Art. 272 longo deveria gerar pelo menos 2 partes, gerou %d", partsOf272)
	}

	// Nenhum chunk deve exceder o limite.
	for _, c := range chunks {
		if len(c.Content) > maxChunkChars {
			t.Errorf("chunk %q excede o limite: %d chars", c.ID, len(c.Content))
		}
	}
}
