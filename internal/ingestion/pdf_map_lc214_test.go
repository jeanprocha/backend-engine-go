package ingestion

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// reArtigoComLetra casa títulos de artigo inseridos por lei posterior:
// "Art. 323-A.", "Art. 7º-A", "Art. 218-A."
var reArtigoComLetra = regexp.MustCompile(`^Art\.\s*\d+[º°]?-[A-Z]+\.?$`)

// A ancoragem "Ver lei" junta duas coisas geradas por ferramentas diferentes:
// o article_id que o parser Go produz do Markdown limpo (ParseArticles) e a
// chave do mapa artigo→página que scripts/legislacao/map_pdf_pages.py produz do
// PDF. ApplyLeiArticlePageMap só faz `continue` quando a chave não bate — ou
// seja, uma divergência de normalização entre as duas regras não dá erro
// nenhum: os chunks simplesmente saem sem pdf_page e o botão "Ver lei" some.
//
// Já aconteceu de verdade: o regex do Python não capturava o sufixo de letra e
// gerava a chave "Art. 323" para "Art. 323-A", que o dedup "primeira ocorrência
// vence" então descartava — os 36 artigos com letra da LC 214/2025 ficariam
// sem página nenhuma (W1/Onda 2, PR 4). Este teste é o que impede a regressão.

func lc214Paths(t *testing.T) (md string, mapa string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolver raiz do repo: %v", err)
	}
	return filepath.Join(root, "docs", "lc214_2025_limpa.md"),
		filepath.Join(root, "docs", "legislacao", "lc214_article_page_map.json")
}

func TestLeiArticlePageMapLC214_CobreTodosOsArtigosDoCorpus(t *testing.T) {
	mdPath, mapPath := lc214Paths(t)

	raw, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("ler corpus limpo: %v", err)
	}
	m, err := LoadLeiArticlePageMap(mapPath)
	if err != nil {
		t.Fatalf("carregar mapa: %v", err)
	}

	chunks := NewParserForDocument(string(raw), DocumentProfile{
		IDPrefix: "lc214_", SourceLabel: "LC 214/2025",
	}).ParseArticles()
	if len(chunks) == 0 {
		t.Fatal("o corpus limpo não produziu nenhum artigo")
	}

	// article_id distintos (um artigo longo vira várias partes com o mesmo id).
	ids := map[string]bool{}
	for _, c := range chunks {
		if aid := c.Metadata["article_id"]; aid != "" {
			ids[aid] = true
		}
	}

	var semPagina []string
	for aid := range ids {
		if _, ok := m.Articles[aid]; !ok {
			semPagina = append(semPagina, aid)
		}
	}

	if len(semPagina) > 0 {
		amostra := semPagina
		if len(amostra) > 15 {
			amostra = amostra[:15]
		}
		t.Errorf("%d de %d article_id do corpus não têm entrada no mapa de páginas — "+
			"esses artigos ficam sem ancoragem \"Ver lei\", em silêncio.\n"+
			"Amostra: %v\n"+
			"Se o regex de âncora mudou num lado, mude no outro: "+
			"internal/ingestion/parse.go e scripts/legislacao/map_pdf_pages.py.",
			len(semPagina), len(ids), amostra)
	}
}

// TestLeiArticlePageMapLC214_ArtigosComLetraTemPaginaPropria trava o caso
// específico que motivou a correção: o artigo-base e o inserido por lei
// posterior não podem compartilhar a mesma entrada.
func TestLeiArticlePageMapLC214_ArtigosComLetraTemPaginaPropria(t *testing.T) {
	_, mapPath := lc214Paths(t)
	m, err := LoadLeiArticlePageMap(mapPath)
	if err != nil {
		t.Fatalf("carregar mapa: %v", err)
	}

	base, okBase := m.Articles["Art. 323."]
	comLetra, okLetra := m.Articles["Art. 323-A."]
	if !okBase || !okLetra {
		t.Fatalf("esperava entradas separadas para \"Art. 323.\" (%v) e \"Art. 323-A.\" (%v)", okBase, okLetra)
	}
	if base.Page == comLetra.Page && base.PdfCoordY == comLetra.PdfCoordY {
		t.Errorf("artigo-base e artigo com letra apontam exatamente a mesma posição "+
			"(página %d, y %s) — sinal de que a chave colidiu", base.Page, base.PdfCoordY)
	}

	// A LC 227/2026 inseriu 323-A até 323-M, entre outros; se o sufixo voltar
	// a ser descartado, a contagem despenca. Regex em vez de aritmética de
	// índice porque o título tanto pode terminar em ponto ("Art. 323-A.")
	// quanto não ("Art. 7º-A").
	comLetraTotal := 0
	for aid := range m.Articles {
		if reArtigoComLetra.MatchString(aid) {
			comLetraTotal++
		}
	}
	if comLetraTotal < 30 {
		t.Errorf("só %d artigos com sufixo de letra no mapa; a LC 214/2025 tem 36 — "+
			"o regex de âncora provavelmente parou de capturar o sufixo", comLetraTotal)
	}
}
