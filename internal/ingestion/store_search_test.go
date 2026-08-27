package ingestion

import (
	"strings"
	"testing"
)

// TestSearchQueryFor_SemPrefixoUsaTresArgumentos é a garantia de compatibilidade
// de deploy (W1/Onda 2, PR 1). A migration 009 troca match_tax_law/3 por
// match_tax_law/4 com DEFAULT: a chamada de 3 argumentos resolve nas DUAS
// versões da função, a de 4 só depois de aplicada. Como push no backend
// dispara deploy automático no Railway, emitir sempre 4 argumentos abriria uma
// janela em que o binário novo fala com o banco antigo e toda classificação
// quebra com "function match_tax_law(...) does not exist".
func TestSearchQueryFor_SemPrefixoUsaTresArgumentos(t *testing.T) {
	q := searchQueryFor("")
	if !strings.Contains(q, "match_tax_law($1, $2, $3)") {
		t.Errorf("sem prefixo deveria usar a chamada de 3 argumentos; got:\n%s", q)
	}
	if strings.Contains(q, "$4") {
		t.Errorf("sem prefixo a query não pode referenciar $4; got:\n%s", q)
	}
}

func TestSearchQueryFor_ComPrefixoUsaQuatroArgumentos(t *testing.T) {
	q := searchQueryFor("lc214_")
	if !strings.Contains(q, "match_tax_law($1, $2, $3, $4)") {
		t.Errorf("com prefixo deveria usar a chamada de 4 argumentos; got:\n%s", q)
	}
}

// TestSearchQueries_MesmasColunas: as duas formas precisam projetar exatamente
// as mesmas colunas, na mesma ordem — o Scan em Search é único e posicional
// (article_id, content, metadata, similarity). Divergir aqui daria erro de
// tipo em runtime só na configuração que tiver prefixo, que é justamente a
// que os testes de integração não cobrem hoje.
func TestSearchQueries_MesmasColunas(t *testing.T) {
	const projecao = "SELECT article_id, content, metadata, similarity"
	for name, q := range map[string]string{"3 args": searchQuery3, "4 args": searchQuery4} {
		if !strings.Contains(q, projecao) {
			t.Errorf("%s: projeção divergente do Scan posicional de Search; got:\n%s", name, q)
		}
	}
}

// TestSearchQueryFor_EspacoEmBrancoNaoContaComoPrefixo documenta o contrato de
// Search: o TrimSpace acontece antes, então " " nunca chega aqui como filtro.
// A função da migration também trata ” como "sem filtro" — defesa em
// profundidade, porque um prefixo vazio filtraria com LIKE '%' e devolveria
// tudo, o que é inofensivo mas mascararia um erro de configuração.
func TestSearchQueryFor_EspacoEmBrancoNaoContaComoPrefixo(t *testing.T) {
	if got := searchQueryFor(strings.TrimSpace("   ")); got != searchQuery3 {
		t.Error("prefixo só de espaços deveria cair na forma de 3 argumentos")
	}
}
