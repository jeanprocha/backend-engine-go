package ingestion

import "testing"

// Artigos com sufixo de letra ("Art. 323-A") são artigos INSERIDOS por lei
// posterior — na LC 214/2025 consolidada, a LC 227/2026 inseriu 57 deles.
// Antes da Onda 2/PR 3 o regex de âncora capturava só os dígitos, então
// "Art. 323-A" era titulado "Art. 323": dois dispositivos distintos
// reivindicando a mesma citação, em 32 números-base. Para um produto cuja tese
// é citação auditável, isso é desqualificante — daí estes testes.

func titlesOf(chunks []ArticleChunk) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, c.Title)
	}
	return out
}

func TestParseArticles_SufixoDeLetraPreservadoNoTitulo(t *testing.T) {
	md := "" +
		"#### Art. 323. Ato conjunto do Comitê deverá ser observado.\n\n" +
		"#### Art. 323-A. É assegurado ao sujeito passivo o direito de consulta.\n\n" +
		"#### Art. 323-B. A solução de consulta será emitida pelo CGIBS.\n"

	p := NewParserForDocument(md, DocumentProfile{IDPrefix: "lc214_", SourceLabel: "LC 214/2025"})
	chunks := p.ParseArticles()

	if len(chunks) != 3 {
		t.Fatalf("esperava 3 artigos, veio %d: %v", len(chunks), titlesOf(chunks))
	}
	want := []string{"Art. 323.", "Art. 323-A.", "Art. 323-B."}
	for i, w := range want {
		if chunks[i].Title != w {
			t.Errorf("chunk %d: título %q, esperado %q", i, chunks[i].Title, w)
		}
		if chunks[i].Metadata["article_id"] != w {
			t.Errorf("chunk %d: metadata.article_id %q, esperado %q", i, chunks[i].Metadata["article_id"], w)
		}
	}
}

// TestParseArticles_SufixoComOrdinal cobre a outra forma que a LC 214 usa:
// o ordinal vem ANTES do hífen ("Art. 7º-A"), não depois.
func TestParseArticles_SufixoComOrdinal(t *testing.T) {
	md := "" +
		"#### Art. 7º Na hipótese de fornecimento de diferentes bens.\n\n" +
		"#### Art. 7º-A. Caso seja possível a aplicação de mais de um instituto.\n"

	chunks := NewParserForDocument(md, DocumentProfile{IDPrefix: "lc214_", SourceLabel: "LC 214/2025"}).ParseArticles()
	if len(chunks) != 2 {
		t.Fatalf("esperava 2 artigos, veio %d: %v", len(chunks), titlesOf(chunks))
	}
	if chunks[0].Title != "Art. 7º" {
		t.Errorf("título do primeiro: %q, esperado \"Art. 7º\"", chunks[0].Title)
	}
	if chunks[1].Title != "Art. 7º-A." {
		t.Errorf("título do segundo: %q, esperado \"Art. 7º-A.\"", chunks[1].Title)
	}
}

// TestParseArticles_IDsDistintosParaArtigoBaseEComLetra é o teste que fecha o
// bug de verdade: os IDs de chunk precisam diferir, senão a citação do dossiê
// aponta o dispositivo errado. sanitizeID transforma "Art. 323-A" em
// "art_323_a" e "Art. 323" em "art_323".
func TestParseArticles_IDsDistintosParaArtigoBaseEComLetra(t *testing.T) {
	md := "#### Art. 323. Caput do artigo base.\n\n#### Art. 323-A. Caput do artigo inserido.\n"
	chunks := NewParserForDocument(md, DocumentProfile{IDPrefix: "lc214_", SourceLabel: "LC 214/2025"}).ParseArticles()

	if len(chunks) != 2 {
		t.Fatalf("esperava 2 artigos, veio %d", len(chunks))
	}
	if chunks[0].ID == chunks[1].ID {
		t.Fatalf("IDs colidiram: ambos %q", chunks[0].ID)
	}
	if got := chunks[1].ID; got != "lc214_0002_art_323_a" {
		t.Errorf("ID do artigo com letra: %q, esperado \"lc214_0002_art_323_a\"", got)
	}
}

// TestParseArticles_RegressaoZeroSemSufixo garante que o corpus ingerido hoje
// (LC 68/2024, zero âncoras com letra) não muda de titulação nem de ID. Se este
// teste quebrar, a mudança do regex deixou de ser inócua para o corpus vivo e a
// re-ingestão passa a ser obrigatória.
func TestParseArticles_RegressaoZeroSemSufixo(t *testing.T) {
	md := "" +
		"#### Art. 1º Ficam instituídos os tributos.\n\n" +
		"#### Art. 52. O contribuinte apropriará créditos.\n\n" +
		"#### Art. 156 Compete aos Municípios.\n"

	chunks := NewParserForDocument(md, DocumentProfile{IDPrefix: "lc68_", SourceLabel: "LC 68/2024"}).ParseArticles()
	if len(chunks) != 3 {
		t.Fatalf("esperava 3 artigos, veio %d: %v", len(chunks), titlesOf(chunks))
	}
	casos := []struct{ title, id string }{
		{"Art. 1º", "lc68_0001_art_1"},
		{"Art. 52.", "lc68_0002_art_52"},
		{"Art. 156", "lc68_0003_art_156"},
	}
	for i, c := range casos {
		if chunks[i].Title != c.title {
			t.Errorf("chunk %d: título %q, esperado %q", i, chunks[i].Title, c.title)
		}
		if chunks[i].ID != c.id {
			t.Errorf("chunk %d: ID %q, esperado %q", i, chunks[i].ID, c.id)
		}
	}
}

// TestParseArticles_NaoAncoraReferenciaInline: "art. 323-H" citado no meio de
// uma frase não pode virar artigo. O (?m)^#+ e o "A" maiúsculo já garantiam
// isso; o sufixo de letra não pode ter afrouxado a regra.
func TestParseArticles_NaoAncoraReferenciaInline(t *testing.T) {
	md := "#### Art. 323-I. A proposição de que trata o inciso I do caput do art. 323-H deverá estar acompanhada.\n"
	chunks := NewParserForDocument(md, DocumentProfile{IDPrefix: "lc214_", SourceLabel: "LC 214/2025"}).ParseArticles()
	if len(chunks) != 1 {
		t.Fatalf("referência inline virou artigo: %d chunks — %v", len(chunks), titlesOf(chunks))
	}
	if chunks[0].Title != "Art. 323-I." {
		t.Errorf("título: %q, esperado \"Art. 323-I.\"", chunks[0].Title)
	}
}
