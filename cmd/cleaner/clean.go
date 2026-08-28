package main

import (
	"regexp"
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
	rePlanaltoCompilado = regexp.MustCompile(`(?im)^.*Texto\s+compilado.*$`)

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
	// TribIA precisa citar (ver os TODO(W1-onda2) em internal/tax/transition_table.go).
	//
	// A regra some em silêncio: nada no pipeline acusa artigo faltando. Foi
	// encontrada comparando a contagem de âncoras do Markdown limpo com a
	// contagem de artigos do texto bruto — daí o teste de arquivo-ouro.
	rePlanaltoVigencia = regexp.MustCompile(`(?im)^\s*Vig[êe]ncia\s*$`)
)

func PlanaltoDOUProfile() Profile {
	return Profile{
		Name:                "planalto-dou",
		SkipParagraphReflow: true,
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

// Clean converte o texto bruto (extraído de PDF/HTML) no Markdown estruturado
// que o parser de ingestão espera (âncoras "#### Art. N"). Pura e testável —
// sem I/O.
func Clean(text string, p Profile) string {
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

	// 4. Markdown com âncoras "#### Art. N" para o parser de ingest.
	var out strings.Builder
	for _, line := range cleanedLines {
		if reArtigoAnchor.MatchString(line) {
			out.WriteString("\n\n#### " + line + "\n")
		} else {
			out.WriteString(line + "\n")
		}
	}
	return out.String()
}

var reMultiSpace = regexp.MustCompile(`[ \t]{2,}`)
