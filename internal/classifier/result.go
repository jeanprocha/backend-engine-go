package classifier

// ClassificationResult é o resultado consolidado da classificação de uma despesa.
// IsEligible indica se a despesa gera crédito de IBS/CBS segundo os artigos recuperados.
// Evidence carrega os artigos que embasaram a decisão, para rastreabilidade.
type ClassificationResult struct {
	IsEligible    bool
	Confidence    float64
	Justification string
	LegalBase     string
	RiskLevel     string // "baixo" | "medio" | "alto"
	Evidence      []EvidenceArticle
}

// EvidenceArticle é um chunk da lei recuperado pelo RAG que sustenta a classificação.
type EvidenceArticle struct {
	ArticleID  string
	Content    string
	Similarity float64
}
