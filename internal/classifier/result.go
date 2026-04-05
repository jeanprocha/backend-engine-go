package classifier

// ClassificationResult é o resultado consolidado da classificação de uma despesa ou serviço.
// IsEligible indica se o item gera crédito de IBS/CBS segundo os artigos recuperados.
// RegimeType indica o regime tributário do item conforme LC 68/2024.
// Evidence carrega os artigos que embasaram a decisão, para rastreabilidade.
type ClassificationResult struct {
	IsEligible    bool
	Confidence    float64
	Justification string
	LegalBase     string
	RiskLevel     string // "baixo" | "medio" | "alto"
	// RegimeType determina a alíquota efetiva de CBS/IBS.
	// "padrao" | "diferenciado_60" (saúde, educação) | "reduzido_zero" (cesta básica).
	RegimeType string
	Evidence   []EvidenceArticle
}

// EvidenceArticle é um chunk da lei recuperado pelo RAG que sustenta a classificação.
type EvidenceArticle struct {
	ArticleID  string
	Content    string
	Similarity float64
}
