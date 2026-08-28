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

// Corte de escopo (Onda 2/PR 5): os 29 ANEXOS da LC 214/2025 não têm âncora de
// artigo, então o parser os absorvia no último artigo — o Art. 544 saía com
// 99.904 caracteres, dos quais só 1.335 são o artigo de verdade. Ingerir assim
// rotularia texto de anexo como "Art. 544". Decisão do usuário: excluir do
// corpus (são listas de NCM/NBS sobre bens; o TribIA modela serviços).
func TestPlanaltoDOU_CortaNoPrimeiroAnexo(t *testing.T) {
	entrada := strings.Join([]string{
		"Art. 543. Ficam revogados os dispositivos a seguir.",
		"Art. 544. Esta Lei Complementar entra em vigor na data de sua publicação.",
		"ANEXO I",
		"Arroz das subposições 1006.20 e 1006.30.",
		"ANEXO II",
		"Leite em pó, conforme a legislação específica.",
	}, "\n")

	saida := Clean(entrada, PlanaltoDOUProfile())

	if !strings.Contains(saida, "#### Art. 544.") {
		t.Error("o último artigo real foi cortado junto com os anexos")
	}
	if !strings.Contains(saida, "entra em vigor na data de sua publicação") {
		t.Error("o caput do último artigo foi perdido")
	}
	if strings.Contains(saida, "ANEXO") {
		t.Errorf("cabeçalho de anexo sobreviveu ao corte.\nSaída:\n%s", saida)
	}
	if strings.Contains(saida, "Arroz das subposições") || strings.Contains(saida, "Leite em pó") {
		t.Errorf("conteúdo de anexo entrou no corpus.\nSaída:\n%s", saida)
	}
}

// O corte é sensível a maiúsculas de propósito: o texto legal REFERENCIA
// "Anexo XII" (capitalizado) dentro dos artigos, e cortar ali decapitaria a
// lei no meio. Verificado na LC 214: 4 menções inline, nenhuma no início de
// linha, e 29 cabeçalhos em caixa alta, todos depois do último artigo.
func TestPlanaltoDOU_ReferenciaInlineAAnexoNaoCorta(t *testing.T) {
	entrada := strings.Join([]string{
		"Art. 10. Aplica-se a alíquota reduzida aos bens listados:",
		"I - no Anexo XII desta Lei Complementar;",
		"II - no Anexo IV desta Lei Complementar.",
		"Art. 11. O disposto no artigo anterior não se aplica a serviços.",
	}, "\n")

	saida := Clean(entrada, PlanaltoDOUProfile())

	if got := strings.Count(saida, "#### Art."); got != 2 {
		t.Errorf("esperava 2 artigos, veio %d — a menção inline a 'Anexo' disparou o corte.\nSaída:\n%s", got, saida)
	}
	if !strings.Contains(saida, "não se aplica a serviços") {
		t.Error("o artigo depois da menção inline foi cortado")
	}
}

// Texto compilado exibe as DUAS redações do artigo alterado: a superada
// primeiro, a vigente depois com "(Redação dada pela ...)". Sem tratar, o
// corpus fica com dois chunks para o mesmo article_id, um deles lei revogada e
// nada distinguindo os dois na recuperação. Medido na LC 214/2025: 21 artigos.
// O Art. 146 é o caso exemplar — a redação superada condiciona alíquota zero a
// uma lista do Anexo XIV, a vigente a registro na Anvisa.
func TestPlanaltoDOU_MantemApenasARedacaoVigente(t *testing.T) {
	entrada := strings.Join([]string{
		"Art. 146. Ficam reduzidas a zero as alíquotas sobre os medicamentos relacionados no Anexo XIV.",
		"Art. 146. São reduzidas a zero as alíquotas sobre os medicamentos registrados na Anvisa: (Redação dada pela Lei Complementar nº 227, de 2026)",
		"Art. 147. Artigo seguinte, sem alteração.",
	}, "\n")

	saida, report := CleanWithReport(entrada, PlanaltoDOUProfile())

	if strings.Contains(saida, "Anexo XIV") {
		t.Errorf("a redação SUPERADA sobreviveu — o corpus citaria lei revogada.\nSaída:\n%s", saida)
	}
	if !strings.Contains(saida, "registrados na Anvisa") {
		t.Errorf("a redação VIGENTE foi removida.\nSaída:\n%s", saida)
	}
	if got := strings.Count(saida, "#### Art. 146."); got != 1 {
		t.Errorf("esperava 1 âncora para o Art. 146, veio %d", got)
	}
	if !strings.Contains(saida, "#### Art. 147.") {
		t.Error("o artigo seguinte foi removido junto")
	}
	if len(report.ArtigosSuperadosRemovidos) != 1 || report.ArtigosSuperadosRemovidos[0] != "Art. 146" {
		t.Errorf("relatório não registrou a remoção: %+v", report.ArtigosSuperadosRemovidos)
	}
}

// A trava: se a última ocorrência NÃO traz a marca de redação vigente, a
// premissa "a última é a atual" não se confirmou — manter tudo e avisar, em vez
// de apagar texto legal por suposição.
func TestPlanaltoDOU_DuplicataSemMarcaNaoEhRemovida(t *testing.T) {
	entrada := strings.Join([]string{
		"Art. 200. Primeira redação, sem marca nenhuma.",
		"Art. 200. Segunda redação, também sem marca.",
	}, "\n")

	saida, report := CleanWithReport(entrada, PlanaltoDOUProfile())

	if got := strings.Count(saida, "#### Art. 200."); got != 2 {
		t.Errorf("esperava as 2 ocorrências mantidas, veio %d — apagou texto legal por suposição", got)
	}
	if len(report.ArtigosSuperadosRemovidos) != 0 {
		t.Errorf("não deveria ter removido nada: %+v", report.ArtigosSuperadosRemovidos)
	}
	if len(report.ArtigosDuplicadosMantidos) != 1 {
		t.Errorf("o caso deveria estar no relatório para revisão: %+v", report.ArtigosDuplicadosMantidos)
	}
}

// Artigo que aparece uma única vez, com a marca, não é afetado — a marca por si
// só não pode disparar remoção.
func TestPlanaltoDOU_ArtigoUnicoComMarcaSobrevive(t *testing.T) {
	entrada := "Art. 300. Redação única. (Redação dada pela Lei Complementar nº 227, de 2026)"
	saida, report := CleanWithReport(entrada, PlanaltoDOUProfile())
	if !strings.Contains(saida, "#### Art. 300.") {
		t.Errorf("artigo único removido.\nSaída:\n%s", saida)
	}
	if len(report.ArtigosSuperadosRemovidos) != 0 {
		t.Errorf("nada deveria ter sido removido: %+v", report.ArtigosSuperadosRemovidos)
	}
}

// Perfis sem CorpusStopAtRe não podem passar a cortar nada.
func TestCamaraPLP_NaoTemCorteDeEscopo(t *testing.T) {
	if CamaraPLPProfile().CorpusStopAtRe != nil {
		t.Error("o perfil da Câmara ganhou um corte de escopo que ninguém pediu")
	}
	entrada := "Art. 1º Primeiro artigo.\nANEXO I\nConteúdo que deve sobreviver."
	if saida := Clean(entrada, CamaraPLPProfile()); !strings.Contains(saida, "deve sobreviver") {
		t.Errorf("o perfil da Câmara cortou no anexo.\nSaída:\n%s", saida)
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
