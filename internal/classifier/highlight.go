package classifier

import (
	"strings"
	"unicode/utf8"
)

const (
	maxEvidenceHighlightEntries = 8
	maxSnippetsPerKind          = 5
	minSnippetRunes             = 2
	// maxSnippetRunes: alvo de realce cirúrgico; trechos da LLM mais longos são truncados
	// para manter o Raio-X legível (ver prompt e TestClipSnippetToMaxRunes).
	maxSnippetRunes = 180
)

// EvidenceHighlightEntry corresponde a um elemento de evidence_highlights na resposta JSON da LLM.
type EvidenceHighlightEntry struct {
	EvidenceIndex     int      `json:"evidence_index"`
	Snippets          []string `json:"snippets"`
	SnippetsTentative []string `json:"snippets_tentative"`
}

// applyEvidenceHighlights preenche RelevantSnippets nos EvidenceArticle com substrings
// que existem literalmente no Content; termos que não aparecem são descartados.
func applyEvidenceHighlights(llm *classificationLLMResponse, evidence []EvidenceArticle) {
	if llm == nil || len(evidence) == 0 || len(llm.EvidenceHighlights) == 0 {
		return
	}
	n := len(llm.EvidenceHighlights)
	if n > maxEvidenceHighlightEntries {
		n = maxEvidenceHighlightEntries
	}
	for _, h := range llm.EvidenceHighlights[:n] {
		if h.EvidenceIndex < 1 || h.EvidenceIndex > len(evidence) {
			continue
		}
		i := h.EvidenceIndex - 1
		content := evidence[i].Content
		evidence[i].RelevantSnippets = filterSnippetsInContent(content, h.Snippets)
		evidence[i].RelevantSnippetsTentative = filterSnippetsInContent(content, h.SnippetsTentative)
	}
}

func filterSnippetsInContent(content string, terms []string) []string {
	if content == "" || len(terms) == 0 {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, t := range terms {
		if len(out) >= maxSnippetsPerKind {
			break
		}
		m := extractMatchInContent(content, t)
		if m == "" {
			continue
		}
		m = clipSnippetToMaxRunes(m, maxSnippetRunes)
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// clipSnippetToMaxRunes devolve o prefixo de s com no máximo maxRunes runes.
// Preserva o alinhamento com o content original (prefixo de uma substring válida continua a ser substring).
func clipSnippetToMaxRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes])
}

func extractMatchInContent(content, term string) string {
	term = strings.TrimSpace(term)
	if len([]rune(term)) < minSnippetRunes {
		return ""
	}
	if strings.Index(content, term) >= 0 {
		return term
	}
	lc := strings.ToLower(content)
	lt := strings.ToLower(term)
	j := strings.Index(lc, lt)
	if j < 0 {
		return ""
	}
	end := j + len(term)
	if end > len(content) {
		return ""
	}
	// Índices alinham-se quando o match difere só por maiúsculas (texto jurídico típico).
	return content[j:end]
}
