package classifier

import (
	"strings"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

// TestBuildSystemPrompt_SubstitutesLawLabel prova que trocar o rótulo muda
// SÓ o rótulo — não usa uma cópia dourada do texto inteiro (o prompt evolui
// com frequência; um golden byte-a-byte viraria manutenção constante e
// desconectada do que este teste realmente precisa proteger).
func TestBuildSystemPrompt_SubstitutesLawLabel(t *testing.T) {
	a := buildSystemPrompt("LC 68/2024")
	if !strings.Contains(a, "LC 68/2024") {
		t.Fatal("esperava o rótulo no prompt")
	}
	if strings.Contains(a, "{{LAW}}") {
		t.Fatal("placeholder {{LAW}} vazou sem substituição")
	}

	b := buildSystemPrompt("LC 214/2025")
	if strings.Contains(b, "LC 68/2024") {
		t.Error("buildSystemPrompt(\"LC 214/2025\") não deveria conter LC 68/2024")
	}
	if !strings.Contains(b, "LC 214/2025") {
		t.Error("esperava LC 214/2025 no prompt")
	}

	// O resto do texto — regras 1 a 17, SOP, schema — não pode diferir.
	normalized := strings.ReplaceAll(a, "LC 68/2024", "LC 214/2025")
	if normalized != b {
		t.Error("buildSystemPrompt deveria diferir SÓ no rótulo da lei entre as duas chamadas")
	}
}

func TestBuildSystemPrompt_PreservesArticleAndAnexoNumbers(t *testing.T) {
	// Art. 131 e Anexo I são números de dispositivo, não nome de lei — não
	// devem mudar com o rótulo (validar a numeração é trabalho da Onda 2).
	p := buildSystemPrompt("LC 214/2025")
	if !strings.Contains(p, "Art. 131") {
		t.Error("Art. 131 não deveria ter sido alterado/removido")
	}
	if !strings.Contains(p, "Anexo I da LC 214/2025") {
		t.Error("esperava \"Anexo I da LC 214/2025\" — só o rótulo muda, o número do anexo fica")
	}
}

func TestBuildLeakageSOP_SubstitutesLawLabel(t *testing.T) {
	a := buildLeakageSOP("LC 68/2024")
	b := buildLeakageSOP("LC 214/2025")
	if strings.Contains(a, "{{LAW}}") || strings.Contains(b, "{{LAW}}") {
		t.Fatal("placeholder {{LAW}} vazou sem substituição")
	}
	if strings.ReplaceAll(a, "LC 68/2024", "LC 214/2025") != b {
		t.Error("buildLeakageSOP deveria diferir SÓ no rótulo da lei")
	}
}

func TestBuildExpandQueryPrompt_SubstitutesLawLabel(t *testing.T) {
	a := buildExpandQueryPrompt("LC 68/2024")
	b := buildExpandQueryPrompt("LC 214/2025")
	if strings.Contains(a, "{{LAW}}") || strings.Contains(b, "{{LAW}}") {
		t.Fatal("placeholder {{LAW}} vazou sem substituição")
	}
	if strings.ReplaceAll(a, "LC 68/2024", "LC 214/2025") != b {
		t.Error("buildExpandQueryPrompt deveria diferir SÓ no rótulo da lei")
	}
}

func TestBuildUserMessage_DerivesLawLabelFromArticles(t *testing.T) {
	articles := []ingestion.SearchResult{
		{ArticleID: "lc214_0001_art_1", Content: "texto", Metadata: map[string]string{"source": "LC 214/2025"}},
	}
	msg := buildUserMessage("AWS", "", "", articles)
	if !strings.Contains(msg, "CONTEXTO JURÍDICO (artigos recuperados da LC 214/2025):") {
		t.Errorf("esperava o rótulo derivado do chunk no cabeçalho do contexto jurídico: %q", msg)
	}
}

func TestBuildUserMessage_FallsBackToDefaultWhenNoSource(t *testing.T) {
	articles := []ingestion.SearchResult{
		{ArticleID: "lc68_0001_art_1", Content: "texto", Metadata: map[string]string{}},
	}
	msg := buildUserMessage("AWS", "", "", articles)
	want := "CONTEXTO JURÍDICO (artigos recuperados da " + defaultLawLabel() + "):"
	if !strings.Contains(msg, want) {
		t.Errorf("esperava fallback para o rótulo default: %q", msg)
	}
}

func TestLawLabelFromArticles(t *testing.T) {
	cases := []struct {
		name     string
		articles []ingestion.SearchResult
		want     string
	}{
		{"sem artigos usa default", nil, defaultLawLabel()},
		{
			"um artigo com source",
			[]ingestion.SearchResult{{ArticleID: "a", Metadata: map[string]string{"source": "LC 214/2025"}}},
			"LC 214/2025",
		},
		{
			"mistura de documentos — usa o primeiro",
			[]ingestion.SearchResult{
				{ArticleID: "a", Metadata: map[string]string{"source": "LC 214/2025"}},
				{ArticleID: "b", Metadata: map[string]string{"source": "LC 68/2024"}},
			},
			"LC 214/2025",
		},
		{
			"sem metadata source no primeiro, com no segundo",
			[]ingestion.SearchResult{
				{ArticleID: "a", Metadata: map[string]string{}},
				{ArticleID: "b", Metadata: map[string]string{"source": "LC 214/2025"}},
			},
			"LC 214/2025",
		},
		{
			"nenhum artigo com source → default",
			[]ingestion.SearchResult{{ArticleID: "a", Metadata: map[string]string{}}},
			defaultLawLabel(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lawLabelFromArticles(tc.articles)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
