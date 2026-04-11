package ingestion

import (
	"regexp"
	"strings"
)

const structureVersion = "1"

// LegalPathSummary descreve o dispositivo principal inferido de forma determinística
// a partir do texto do chunk (sem LLM).
type LegalPathSummary struct {
	ArticleLabel string
	Paragraph    string // ex.: "§ 1º", "Parágrafo único", ou vazio (caput)
	Inciso       string // ex.: "I", "II"
	Alinea       string // ex.: "a", "b"
	SpanNote     string // vazio ou aviso quando o trecho não permite um único dispositivo
}

var (
	reParagrafoNum   = regexp.MustCompile(`(?i)^\s*§\s*(\d+)\s*([º°])?`)
	reParagrafoUnico = regexp.MustCompile(`(?i)^\s*Parágrafo\s+único\.?`)
	// Incisos romanos no início de linha: I -, II –, XI –
	reInciso = regexp.MustCompile(`^\s*([IVXLC]{1,8})\s*[-–—]\s*`)
	// Alíneas: a) b)
	reAlinea = regexp.MustCompile(`^\s*([a-z])\)\s*`)
)

// caputEParagrafos é verdadeiro quando o trecho contém incisos do caput (antes do primeiro §/PU)
// e também um parágrafo numerado ou único — mistura que não deve receber um único ponteiro.
func caputEParagrafos(lines []string) bool {
	var sawPara bool
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if reParagrafoNum.MatchString(line) || reParagrafoUnico.MatchString(line) {
			sawPara = true
			break
		}
	}
	if !sawPara {
		return false
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if reParagrafoNum.MatchString(line) || reParagrafoUnico.MatchString(line) {
			break
		}
		if reInciso.MatchString(line) {
			return true
		}
	}
	return false
}

// AnalyzeLegalPath percorre o conteúdo do chunk linha a linha e infere o dispositivo
// principal quando isso é possível sem ambiguidade relevante.
func AnalyzeLegalPath(articleLabel, content string) LegalPathSummary {
	summary := LegalPathSummary{ArticleLabel: strings.TrimSpace(articleLabel)}
	content = strings.TrimSpace(content)
	if content == "" {
		return summary
	}

	lines := strings.Split(content, "\n")
	if caputEParagrafos(lines) {
		return LegalPathSummary{
			ArticleLabel: summary.ArticleLabel,
			SpanNote:     "trecho caput e parágrafos",
		}
	}

	var (
		para              string
		firstInc, firstAli string
		secNum            string // último número de § visto ("1", "2", ...)
		multiSec          bool
		incisoLines       int
	)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if reParagrafoUnico.MatchString(line) {
			if secNum != "" && secNum != "PU" {
				multiSec = true
			}
			secNum = "PU"
			para = "Parágrafo único"
			firstInc, firstAli = "", ""
			continue
		}
		if m := reParagrafoNum.FindStringSubmatch(line); m != nil {
			n := m[1]
			if secNum != "" && secNum != "PU" && secNum != n {
				multiSec = true
			}
			secNum = n
			para = "§ " + n + "º"
			firstInc, firstAli = "", ""
			continue
		}
		if m := reInciso.FindStringSubmatch(line); m != nil {
			incisoLines++
			if firstInc == "" {
				firstInc = m[1]
			}
			continue
		}
		if m := reAlinea.FindStringSubmatch(line); m != nil {
			if firstAli == "" {
				firstAli = m[1]
			}
			continue
		}
	}

	if multiSec {
		return LegalPathSummary{
			ArticleLabel: summary.ArticleLabel,
			SpanNote:     "trecho abrange múltiplos parágrafos",
		}
	}

	// Caput com muitos incisos e texto longo: não forçar inciso único.
	if secNum == "" && incisoLines > 6 && len(content) > 2000 {
		return LegalPathSummary{
			ArticleLabel: summary.ArticleLabel,
			SpanNote:     "trecho abrange múltiplos incisos do caput",
		}
	}

	summary.Paragraph = para
	summary.Inciso = firstInc
	summary.Alinea = firstAli
	return summary
}

// LegalPathMetadataKeys são as chaves persistidas em metadata JSON (tax_law_chunks).
const (
	MetaArticleLabel     = "article_label"
	MetaParagraph        = "paragraph"
	MetaInciso           = "inciso"
	MetaAlinea           = "alinea"
	MetaSpanNote         = "span_note"
	MetaStructureVersion = "structure_version"
)

// ApplyLegalPathToMetadata mescla article_label, paragraph, inciso, alinea, span_note e structure_version
// num mapa de metadados existente (source, type, article_id, etc.).
func ApplyLegalPathToMetadata(meta map[string]string, path LegalPathSummary) {
	if meta == nil {
		return
	}
	if al := strings.TrimSpace(path.ArticleLabel); al != "" {
		meta[MetaArticleLabel] = al
	}
	if path.SpanNote != "" {
		meta[MetaSpanNote] = path.SpanNote
		meta[MetaParagraph] = ""
		meta[MetaInciso] = ""
		meta[MetaAlinea] = ""
		meta[MetaStructureVersion] = structureVersion
		return
	}
	if p := strings.TrimSpace(path.Paragraph); p != "" {
		meta[MetaParagraph] = p
	} else {
		meta[MetaParagraph] = ""
	}
	if i := strings.TrimSpace(path.Inciso); i != "" {
		meta[MetaInciso] = i
	} else {
		meta[MetaInciso] = ""
	}
	if a := strings.TrimSpace(path.Alinea); a != "" {
		meta[MetaAlinea] = a
	} else {
		meta[MetaAlinea] = ""
	}
	meta[MetaStructureVersion] = structureVersion
}

// FormatLegalCitation monta uma única string de referência a partir dos metadados
// (determinística; usada no servidor para Opção A).
func FormatLegalCitation(meta map[string]string) string {
	if meta == nil {
		return ""
	}
	al := strings.TrimSpace(meta[MetaArticleLabel])
	if al == "" {
		al = strings.TrimSpace(meta["article_id"])
	}
	if al == "" {
		return ""
	}
	if sn := strings.TrimSpace(meta[MetaSpanNote]); sn != "" {
		return al + " (" + sn + ")"
	}
	var parts []string
	parts = append(parts, al)
	if p := strings.TrimSpace(meta[MetaParagraph]); p != "" {
		parts = append(parts, p)
	}
	if i := strings.TrimSpace(meta[MetaInciso]); i != "" {
		parts = append(parts, "inciso "+i)
	}
	if a := strings.TrimSpace(meta[MetaAlinea]); a != "" {
		parts = append(parts, "alínea "+a+")")
	}
	return strings.Join(parts, ", ")
}
