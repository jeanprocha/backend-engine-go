package classifier

import (
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

// applyDeterministicCitation reconstrói legal_base a partir dos metadados do chunk
// referenciado (Opção A). primary_evidence_index é 1-based; se inválido, risk_level alto.
func applyDeterministicCitation(llm *classificationLLMResponse, evidence []EvidenceArticle) (legalBase string, riskLevel string) {
	riskLevel = strings.TrimSpace(llm.RiskLevel)
	if riskLevel == "" {
		riskLevel = "medio"
	}

	if len(evidence) == 0 {
		return "", riskLevel
	}

	idx := -1
	if llm.PrimaryEvidenceIndex != nil {
		p := *llm.PrimaryEvidenceIndex
		if p < 1 || p > len(evidence) {
			return "", forceRiskAtLeast(riskLevel, "alto")
		}
		idx = p - 1
	} else {
		idx = pickDefaultEvidenceIndex(evidence)
	}

	meta := evidence[idx].Metadata
	legalBase = ingestion.FormatLegalCitation(meta)
	if legalBase == "" {
		legalBase = strings.TrimSpace(evidence[idx].ArticleID)
	}
	return legalBase, riskLevel
}

func pickDefaultEvidenceIndex(evidence []EvidenceArticle) int {
	for i, e := range evidence {
		if !isAnchorArticleID(e.ArticleID) {
			return i
		}
	}
	return 0
}

func forceRiskAtLeast(current, floor string) string {
	order := map[string]int{"baixo": 0, "medio": 1, "alto": 2}
	c := order[strings.ToLower(strings.TrimSpace(current))]
	f := order[floor]
	if c < f {
		return floor
	}
	return current
}
