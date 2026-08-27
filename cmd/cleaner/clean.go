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

// Regras do texto oficial do Planalto (DOU) — deliberadamente PARCIAL.
// São fatos conhecidos sobre a formatação da página do Planalto (cabeçalho
// institucional, rodapés padrão de "texto compilado"/vigência), mas NINGUÉM
// testou isto contra o PDF/HTML real da LC 214/2025 ainda — isso é trabalho
// da Onda 2, contra o arquivo baixado de verdade. Não adicionar regra nova
// aqui "no escuro"; finalizar comparando saída limpa vs. texto original.
var (
	rePlanaltoPresidencia  = regexp.MustCompile(`(?im)^.*Presid[êe]ncia\s+da\s+Rep[úu]blica.*$`)
	rePlanaltoCasaCivil    = regexp.MustCompile(`(?im)^.*Casa\s+Civil.*$`)
	rePlanaltoSecretaria   = regexp.MustCompile(`(?im)^.*Secretaria[\s-]Geral.*$`)
	rePlanaltoSubchefia    = regexp.MustCompile(`(?im)^.*Subchefia\s+para\s+Assuntos\s+Jur[íi]dicos.*$`)
	rePlanaltoNaoSubstitui = regexp.MustCompile(`(?im)^.*[Ee]ste\s+texto\s+n[ãa]o\s+substitui\s+o\s+publicado\s+no\s+DOU.*$`)
	rePlanaltoVeto         = regexp.MustCompile(`(?im)^.*Mensagem\s+de\s+veto.*$`)
	rePlanaltoCompilado    = regexp.MustCompile(`(?im)^.*Texto\s+compilado.*$`)
	rePlanaltoVigencia     = regexp.MustCompile(`(?im)^.*Vig[êe]ncia.*$`)
)

func PlanaltoDOUProfile() Profile {
	return Profile{
		Name: "planalto-dou",
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
	lines := strings.Split(text, "\n")
	var cleanedLines []string
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
