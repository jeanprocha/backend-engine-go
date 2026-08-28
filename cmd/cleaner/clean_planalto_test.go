package main

import (
	"strings"
	"testing"
)

// Regressões do perfil planalto-dou, todas encontradas na Onda 2/PR 3 rodando
// o cleaner contra o HTML real da LC 214/2025. As duas falhas abaixo eram
// SILENCIOSAS — nada no pipeline acusa artigo faltando; só apareceram ao
// comparar a contagem de âncoras do Markdown com a de artigos do texto bruto.

// A regra de ruído "Vigência" era `^.*Vig[êe]ncia.*$` e apagava qualquer linha
// que mencionasse a palavra. Isso destruía 72 linhas da LC 214/2025, entre elas
// três artigos inteiros — e não quaisquer três: são os que definem o cálculo
// das alíquotas de referência de CBS e IBS na transição, que é exatamente o que
// o motor do TribIA modela.
func TestPlanaltoDOU_NaoApagaArtigoQueMencionaVigencia(t *testing.T) {
	entrada := strings.Join([]string{
		"Art. 352. O cálculo da alíquota de referência da CBS para cada ano de vigência de 2027 a 2033 será realizado nos termos dos arts. 353 a 359.",
		"Art. 360. O cálculo das alíquotas de referência estadual e municipal do IBS para cada ano de vigência de 2029 a 2033 será realizado.",
		"Art. 370. O cálculo do redutor a ser aplicado, em cada ano de vigência, sobre as alíquotas da CBS e do IBS.",
	}, "\n")

	saida := Clean(entrada, PlanaltoDOUProfile())

	for _, art := range []string{"#### Art. 352.", "#### Art. 360.", "#### Art. 370."} {
		if !strings.Contains(saida, art) {
			t.Errorf("%s sumiu do texto limpo — a regra de 'Vigência' voltou a ser ampla demais.\nSaída:\n%s", art, saida)
		}
	}
	if !strings.Contains(saida, "alíquota de referência da CBS") {
		t.Error("o caput do Art. 352 foi perdido")
	}
}

// A contrapartida: o link isolado "Vigência" do cabeçalho do Planalto continua
// sendo removido. Sem este caso, "consertar" a regra acima poderia virar
// simplesmente apagá-la.
func TestPlanaltoDOU_RemoveLinkIsoladoDeVigencia(t *testing.T) {
	entrada := "Texto compilado\nVigência\nArt. 1º Ficam instituídos os tributos."
	saida := Clean(entrada, PlanaltoDOUProfile())

	for _, linha := range strings.Split(saida, "\n") {
		if strings.TrimSpace(linha) == "Vigência" {
			t.Error("link isolado 'Vigência' do cabeçalho não foi removido")
		}
	}
	if !strings.Contains(saida, "#### Art. 1º") {
		t.Errorf("o artigo seguinte ao link não sobreviveu.\nSaída:\n%s", saida)
	}
}

// A remontagem de parágrafos junta linhas até achar ".", ":" ou ";". O texto
// consolidado do Planalto tem centenas de linhas terminadas em ")" — as
// anotações "(Incluído pela Lei Complementar nº 227, de 2026)" — e cada uma
// colava o "Art. N" seguinte no fim da linha anterior, que então deixava de
// COMEÇAR com "Art." e perdia a âncora. Media 297 âncoras para 601 artigos.
func TestPlanaltoDOU_NaoRemontaParagrafosEmFonteJaEmLinhas(t *testing.T) {
	entrada := strings.Join([]string{
		"Art. 323. Ato conjunto do Comitê deverá ser observado. (Incluído pela Lei Complementar nº 227, de 2026)",
		"Art. 323-A. É assegurado ao sujeito passivo o direito de consulta. (Incluído pela Lei Complementar nº 227, de 2026)",
		"Art. 323-B. A solução de consulta será emitida pelo CGIBS. (Incluído pela Lei Complementar nº 227, de 2026)",
	}, "\n")

	saida := Clean(entrada, PlanaltoDOUProfile())

	if got := strings.Count(saida, "#### Art."); got != 3 {
		t.Errorf("esperava 3 âncoras, veio %d — a remontagem de parágrafos colou artigos.\nSaída:\n%s", got, saida)
	}
	for _, art := range []string{"#### Art. 323.", "#### Art. 323-A.", "#### Art. 323-B."} {
		if !strings.Contains(saida, art) {
			t.Errorf("%s não virou âncora", art)
		}
	}
}

// O perfil da Câmara (PDF do PLP 68, o corpus ingerido hoje) PRECISA da
// remontagem: lá as quebras de linha no meio da frase são reais. Se este teste
// quebrar, a mudança do SkipParagraphReflow vazou para o perfil errado e uma
// re-ingestão da LC 68 sairia diferente do que está no banco.
func TestCamaraPLP_ContinuaRemontandoParagrafos(t *testing.T) {
	entrada := "Art. 1º Esta Lei Complementar institui o Imposto sobre Bens\ne Serviços e a Contribuição Social sobre Bens e Serviços."
	saida := Clean(entrada, CamaraPLPProfile())

	if !strings.Contains(saida, "Imposto sobre Bens e Serviços") {
		t.Errorf("a linha quebrada no meio da frase não foi remontada.\nSaída:\n%s", saida)
	}
	if got := strings.Count(saida, "#### Art."); got != 1 {
		t.Errorf("esperava 1 âncora, veio %d", got)
	}
}
