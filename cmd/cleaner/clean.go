package main

import (
	"regexp"
	"sort"
	"strings"
)

// Profile descreve o ruído específico de UMA fonte de PDF/HTML — cabeçalho
// institucional, título repetido, rodapé de tramitação. As regras
// compartilhadas (marcador de página, linha só com número, form feed,
// remontagem de parágrafo, ancoragem "Art. N") valem para qualquer norma
// brasileira estruturada em artigos e vivem fora do profile, em Clean.
type Profile struct {
	Name  string
	Noise []*regexp.Regexp

	// SkipParagraphReflow desliga a remontagem de parágrafos (passo 3 de Clean).
	//
	// A remontagem existe para consertar UM defeito específico: PDF quebra
	// linha no meio da frase. Ela junta linhas até encontrar uma que termine em
	// ".", ":" ou ";". Numa fonte que já entrega uma linha por bloco — o HTML
	// do Planalto passado por scripts/legislacao/html_to_text.py — ela deixa de
	// consertar e passa a destruir: a LC 214/2025 tem centenas de linhas
	// terminadas em ")" por causa das anotações "(Incluído pela Lei
	// Complementar nº 227, de 2026)", e cada uma delas cola o "Art. N" seguinte
	// no fim da linha anterior. O artigo deixa de COMEÇAR a linha, perde a
	// âncora "#### Art. N" e some do corpus.
	//
	// Medido na Onda 2/PR 3: com remontagem, 297 âncoras para 601 artigos reais
	// — mais da metade da lei desaparecia em silêncio. É a mesma falha que
	// deixou o "Art. 1º" da LC 68 sem âncora e GET /law/articles/lc68_0001_art_1
	// em 404, aqui em escala.
	SkipParagraphReflow bool

	// CorpusStopAtRe corta o texto na primeira linha que casar — tudo dali em
	// diante fica FORA do corpus.
	//
	// Isto NÃO é regra de ruído: é decisão de ESCOPO do corpus, e por isso está
	// separada de Noise. O que ela resolve na LC 214/2025 (Onda 2/PR 5): os 29
	// ANEXOS da lei não têm âncora de artigo, então o parser os absorve no
	// último artigo — o Art. 544 saía com 99.904 caracteres, dos quais só 1.335
	// são o artigo de verdade (as cláusulas de vigência). Ingerir assim geraria
	// ~24 chunks titulados "Art. 544. (parte N de M)" com texto de anexo dentro,
	// e uma citação de cesta básica sairia como "Art. 544".
	//
	// A escolha de EXCLUIR em vez de ancorar os anexos foi do usuário
	// (27/08/2026): os anexos são listas de NCM/NBS sobre BENS, e o TribIA
	// modela serviços. Melhor não ter do que ter rotulado errado — e nada se
	// perde do repositório, porque o texto bruto commitado continua completo.
	// Promover a "ancorar como dispositivo próprio" é possível depois.
	CorpusStopAtRe *regexp.Regexp

	// RedacaoVigenteRe marca a redação ATUAL de um artigo que foi alterado por
	// lei posterior (ex.: "(Redação dada pela Lei Complementar nº 227, de 2026)").
	//
	// Texto compilado exibe as duas redações do mesmo artigo em sequência: a
	// superada primeiro, a vigente depois com esta anotação. Sem tratar isso, o
	// corpus fica com DOIS chunks para o mesmo article_id — um deles lei
	// revogada, sem nada que a distinga na hora da recuperação. Medido na
	// LC 214/2025: 21 artigos nessa situação. O Art. 146 é o caso exemplar —
	// a redação superada condiciona alíquota zero a uma lista do Anexo XIV, a
	// vigente a registro na Anvisa. Citar a errada é afirmar regra que não
	// existe mais.
	//
	// Quando definido, Clean mantém apenas a ÚLTIMA ocorrência de cada artigo
	// duplicado — e só se ela realmente carregar esta marca. Se não carregar, a
	// premissa "a última é a vigente" não se confirmou naquele documento e nada
	// é removido: o relatório de CleanWithReport avisa, em vez de apagar texto
	// legal com base numa suposição.
	RedacaoVigenteRe *regexp.Regexp
}

// CleanReport registra o que Clean descartou. Corte de escopo e remoção de
// redação superada mudam o conteúdo do corpus — nunca podem ser silenciosos.
type CleanReport struct {
	// ArtigosSuperadosRemovidos: títulos cuja redação antiga saiu do corpus.
	ArtigosSuperadosRemovidos []string
	// ArtigosDuplicadosMantidos: títulos duplicados em que a última ocorrência
	// NÃO trazia a marca de redação vigente — mantidos todos, para revisão.
	ArtigosDuplicadosMantidos []string
}

// Regras compartilhadas — texto legal brasileiro em geral, não uma fonte específica.
var (
	rePaginaMarker   = regexp.MustCompile(`(?im)^.*[Pp][áa]g[ianção\.]*\s*\d+.*$`)
	reLineOnlyNumber = regexp.MustCompile(`(?m)^\s*\d+\s*$`)
	reFormFeed       = regexp.MustCompile(`\f`)
)

func sharedNoise() []*regexp.Regexp {
	return []*regexp.Regexp{rePaginaMarker, reLineOnlyNumber, reFormFeed}
}

// Regras específicas do PDF de tramitação da Câmara dos Deputados (formato
// do PLP 68/2024, hoje em lei-em-texto.txt).
var (
	reCamaraSpaced      = regexp.MustCompile(`(?i)C\s+[AÂ]\s*M\s+A\s+R\s+A\s+D\s+O\s+S\s+D\s+E\s+P\s+U\s+T\s+A\s+D\s+O\s+S`)
	reCamaraCompact     = regexp.MustCompile(`(?im)^.*[CÂ][AÂ]MARA\s+DOS\s+DEPUTADOS.*$`)
	reProjetoLei        = regexp.MustCompile(`(?im)^.*PROJETO\s+DE\s+LEI\s+COMPLEMENTAR.*$`)
	reApresentacaoPLP68 = regexp.MustCompile(`(?i)Apresenta[cç][aã]o:.*?PLP\s*68/2024`)
)

// CamaraPLPProfile é o perfil default — reproduz byte a byte o comportamento
// anterior (era a única fonte suportada). A ORDEM importa: cada regra opera
// sobre o texto já modificado pelas anteriores.
func CamaraPLPProfile() Profile {
	return Profile{
		Name: "camara-plp",
		Noise: []*regexp.Regexp{
			reCamaraSpaced,
			reCamaraCompact,
			reProjetoLei,
			rePaginaMarker,
			reLineOnlyNumber,
			reApresentacaoPLP68,
			reFormFeed,
		},
	}
}

// Regras do texto oficial do Planalto (DOU). Validadas na Onda 2/PR 3 contra o
// HTML real da LC 214/2025 (baixado de planalto.gov.br/ccivil_03/leis/lcp/lcp214.htm,
// 5,4 MB, ISO-8859-1) passado por scripts/legislacao/html_to_text.py.
//
// Este profile pressupõe UMA LINHA POR BLOCO na entrada — é o que o extrator
// HTML produz, e por isso SkipParagraphReflow é true. Para texto extraído de
// PDF do Planalto (que quebra linha no meio da frase) seria preciso um profile
// separado com a remontagem ligada; não existe caso desses hoje.
var (
	rePlanaltoPresidencia  = regexp.MustCompile(`(?im)^.*Presid[êe]ncia\s+da\s+Rep[úu]blica.*$`)
	rePlanaltoCasaCivil    = regexp.MustCompile(`(?im)^.*Casa\s+Civil.*$`)
	rePlanaltoSecretaria   = regexp.MustCompile(`(?im)^.*Secretaria[\s-]Geral.*$`)
	rePlanaltoSubchefia    = regexp.MustCompile(`(?im)^.*Subchefia\s+para\s+Assuntos\s+Jur[íi]dicos.*$`)
	rePlanaltoNaoSubstitui = regexp.MustCompile(`(?im)^.*[Ee]ste\s+texto\s+n[ãa]o\s+substitui\s+o\s+publicado\s+no\s+DOU.*$`)
	rePlanaltoVeto         = regexp.MustCompile(`(?im)^.*Mensagem\s+de\s+veto.*$`)
	rePlanaltoCompilado    = regexp.MustCompile(`(?im)^.*Texto\s+compilado.*$`)

	// rePlanaltoVigencia casa a LINHA ISOLADA "Vigência" — o link do cabeçalho
	// do Planalto —, nunca a palavra no meio do texto.
	//
	// A versão anterior era `^.*Vig[êe]ncia.*$` e apagava QUALQUER linha que
	// mencionasse vigência. Numa lei de transição tributária isso é
	// catastrófico: medido contra a LC 214/2025 (Onda 2/PR 3), destruía 72
	// linhas de texto legal, das quais 3 eram artigos inteiros — Art. 352
	// (cálculo da alíquota de referência da CBS de 2027 a 2033), Art. 360
	// (alíquotas de referência estadual e municipal do IBS de 2029 a 2033) e
	// Art. 370 (redutor sobre as alíquotas nas operações com a administração
	// pública). São precisamente os dispositivos que a tabela de transição do
	// TribIA precisa citar (auditados na Onda 2/W1 — ver internal/tax/transition_table.go).
	//
	// A regra some em silêncio: nada no pipeline acusa artigo faltando. Foi
	// encontrada comparando a contagem de âncoras do Markdown limpo com a
	// contagem de artigos do texto bruto — daí o teste de arquivo-ouro.
	rePlanaltoVigencia = regexp.MustCompile(`(?im)^\s*Vig[êe]ncia\s*$`)
)

// reAnexoHeading casa o cabeçalho de anexo: linha que COMEÇA com "ANEXO" em
// caixa alta seguido de numeral romano. Sensível a maiúsculas de propósito —
// as referências dentro do texto legal escrevem "Anexo XII" (capitalizado) e
// nunca no início da linha, então não disparam o corte. Verificado contra a
// LC 214/2025: 29 cabeçalhos, todos depois do último artigo, e 4 menções
// inline, nenhuma no início de linha.
var reAnexoHeading = regexp.MustCompile(`^\s*ANEXO\s+[IVXLC]+\b`)

// reRedacaoDada marca a redação VIGENTE de um artigo alterado por lei
// posterior — "(Redação dada pela Lei Complementar nº 227, de 2026)".
// Ver Profile.RedacaoVigenteRe.
var reRedacaoDada = regexp.MustCompile(`\(Reda[çc][ãa]o dada pela`)

func PlanaltoDOUProfile() Profile {
	return Profile{
		Name:                "planalto-dou",
		SkipParagraphReflow: true,
		CorpusStopAtRe:      reAnexoHeading,
		RedacaoVigenteRe:    reRedacaoDada,
		Noise: []*regexp.Regexp{
			rePlanaltoPresidencia,
			rePlanaltoCasaCivil,
			rePlanaltoSecretaria,
			rePlanaltoSubchefia,
			rePlanaltoNaoSubstitui,
			rePlanaltoVeto,
			rePlanaltoCompilado,
			rePlanaltoVigencia,
			rePaginaMarker,
			reLineOnlyNumber,
			reFormFeed,
		},
	}
}

// NoneProfile aplica só as regras compartilhadas — escape hatch para uma
// fonte sem perfil dedicado ainda.
func NoneProfile() Profile {
	return Profile{Name: "none", Noise: sharedNoise()}
}

// Profiles é o catálogo por nome, usado pela flag -profile.
func Profiles() map[string]Profile {
	return map[string]Profile{
		"camara-plp":   CamaraPLPProfile(),
		"planalto-dou": PlanaltoDOUProfile(),
		"none":         NoneProfile(),
	}
}

var reAbbrevDot = regexp.MustCompile(`(?i)\b(art|arts|n|nº|inc|par|pars)\.\s*$`)
var reArtigoAnchor = regexp.MustCompile(`^(Art\.\s+\d+)`)

// Clean converte o texto bruto no Markdown estruturado, descartando o
// relatório. Use CleanWithReport quando o que foi removido importar — e ele
// importa em qualquer execução de verdade, porque corte de escopo e remoção de
// redação superada mudam o conteúdo do corpus.
func Clean(text string, p Profile) string {
	out, _ := CleanWithReport(text, p)
	return out
}

// CleanWithReport converte o texto bruto (extraído de PDF/HTML) no Markdown
// estruturado que o parser de ingestão espera (âncoras "#### Art. N") e relata
// o que ficou de fora. Pura e testável — sem I/O.
func CleanWithReport(text string, p Profile) (string, CleanReport) {
	// 1. Ruído do profile, na ordem declarada.
	for _, re := range p.Noise {
		text = re.ReplaceAllString(text, "")
	}

	// 2. Colapsa múltiplos espaços/tabs em um espaço único (antes de remontar parágrafos).
	text = reMultiSpace.ReplaceAllString(text, " ")

	// 3. Reconstrói parágrafos quebrados: PDFs quebram linha no meio da frase;
	// junta linhas que não terminam com pontuação, sem fechar em abreviação jurídica.
	// Fontes que já entregam uma linha por bloco pulam este passo — ver
	// Profile.SkipParagraphReflow para o estrago que ele causa nelas.
	lines := strings.Split(text, "\n")
	var cleanedLines []string

	if p.SkipParagraphReflow {
		for _, line := range lines {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				cleanedLines = append(cleanedLines, trimmed)
			}
		}
	} else {
		var currentLine strings.Builder
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			currentLine.WriteString(trimmed)
			currentLine.WriteString(" ")

			endsWithAbbrev := reAbbrevDot.MatchString(trimmed) ||
				strings.HasSuffix(trimmed, "º.") ||
				strings.HasSuffix(trimmed, "ª.")
			shouldFlush := !endsWithAbbrev &&
				(strings.HasSuffix(trimmed, ".") ||
					strings.HasSuffix(trimmed, ":") ||
					strings.HasSuffix(trimmed, ";"))

			if shouldFlush {
				cleanedLines = append(cleanedLines, currentLine.String())
				currentLine.Reset()
			}
		}
	}

	// 3b. Corte de escopo do corpus (ver Profile.CorpusStopAtRe). Feito DEPOIS
	// da remontagem para que o marcador seja avaliado sobre a linha final, não
	// sobre um fragmento de PDF.
	if p.CorpusStopAtRe != nil {
		for i, line := range cleanedLines {
			if p.CorpusStopAtRe.MatchString(line) {
				cleanedLines = cleanedLines[:i]
				break
			}
		}
	}

	// 3c. Redações superadas (ver Profile.RedacaoVigenteRe).
	cleanedLines, report := removeRedacoesSuperadas(cleanedLines, p)

	// 4. Markdown com âncoras "#### Art. N" para o parser de ingest.
	var out strings.Builder
	for _, line := range cleanedLines {
		if reArtigoAnchor.MatchString(line) {
			out.WriteString("\n\n#### " + line + "\n")
		} else {
			out.WriteString(line + "\n")
		}
	}
	return out.String(), report
}

// tituloCanonico normaliza a âncora de um artigo para agrupar ocorrências do
// MESMO dispositivo ("Art. 146." e "Art. 146" são o mesmo artigo).
func tituloCanonico(line string) string {
	m := reArtigoTitulo.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return "Art. " + m[1] + m[2] + m[3]
}

// reArtigoTitulo espelha o regex de âncora do parser Go
// (internal/ingestion/parse.go) — número, ordinal e sufixo de letra.
var reArtigoTitulo = regexp.MustCompile(`^Art\.\s*(\d+)([º°]?)(-[A-Z]+)?`)

// removeRedacoesSuperadas mantém apenas a última ocorrência de cada artigo
// duplicado, desde que ela traga a marca de redação vigente. Ver
// Profile.RedacaoVigenteRe para o porquê e para a trava.
func removeRedacoesSuperadas(lines []string, p Profile) ([]string, CleanReport) {
	var report CleanReport
	if p.RedacaoVigenteRe == nil {
		return lines, report
	}

	// Blocos: cada âncora de artigo abre um bloco que vai até a próxima.
	type bloco struct{ inicio, fim int } // fim exclusivo
	var blocos []bloco
	var titulos []string
	for i, line := range lines {
		if t := tituloCanonico(line); t != "" && reArtigoAnchor.MatchString(line) {
			if n := len(blocos); n > 0 {
				blocos[n-1].fim = i
			}
			blocos = append(blocos, bloco{inicio: i, fim: len(lines)})
			titulos = append(titulos, t)
		}
	}

	ocorrencias := map[string][]int{}
	for idx, t := range titulos {
		ocorrencias[t] = append(ocorrencias[t], idx)
	}

	descartar := map[int]bool{}
	for titulo, idxs := range ocorrencias {
		if len(idxs) < 2 {
			continue
		}
		ultimo := blocos[idxs[len(idxs)-1]]
		vigente := false
		for _, l := range lines[ultimo.inicio:ultimo.fim] {
			if p.RedacaoVigenteRe.MatchString(l) {
				vigente = true
				break
			}
		}
		if !vigente {
			// Premissa não confirmada: não apagar texto legal por suposição.
			report.ArtigosDuplicadosMantidos = append(report.ArtigosDuplicadosMantidos, titulo)
			continue
		}
		for _, idx := range idxs[:len(idxs)-1] {
			descartar[idx] = true
		}
		report.ArtigosSuperadosRemovidos = append(report.ArtigosSuperadosRemovidos, titulo)
	}

	if len(descartar) == 0 {
		sort.Strings(report.ArtigosDuplicadosMantidos)
		return lines, report
	}

	manter := make([]bool, len(lines))
	for i := range manter {
		manter[i] = true
	}
	for idx := range descartar {
		for i := blocos[idx].inicio; i < blocos[idx].fim; i++ {
			manter[i] = false
		}
	}
	out := make([]string, 0, len(lines))
	for i, keep := range manter {
		if keep {
			out = append(out, lines[i])
		}
	}
	sort.Strings(report.ArtigosSuperadosRemovidos)
	sort.Strings(report.ArtigosDuplicadosMantidos)
	return out, report
}

var reMultiSpace = regexp.MustCompile(`[ \t]{2,}`)
