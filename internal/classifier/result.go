package classifier

// SuggestedTag é uma sugestão opcional da LLM para a base global de chips de contexto (UI).
type SuggestedTag struct {
	Pattern     string
	Label       string
	Category    string
	ColorScheme string
}

// ClassificationResult é o resultado consolidado da classificação de uma despesa ou serviço.
// IsEligible indica se o item gera crédito de IBS/CBS segundo os artigos recuperados.
// RegimeType indica o regime tributário do item conforme a lei (ver systemPromptTemplate).
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
	// SuggestedTags propõe padrões curtos para a base strategy_tags (opcional; pode ser vazio).
	SuggestedTags []SuggestedTag
	// MatchedSpan âncora no contexto da empresa (runas; opcional).
	MatchedSpan *MatchedSpan
}

// EvidenceArticle é um chunk da lei recuperado pelo RAG que sustenta a classificação.
type EvidenceArticle struct {
	ArticleID  string
	Content    string
	Similarity float64
	Metadata   map[string]string
	// RelevantSnippets e RelevantSnippetsTentative são substrings validadas no Content (Go); a UI PRO realça.
	RelevantSnippets          []string
	RelevantSnippetsTentative []string
}

// CreditLeakEnrichmentItem é o payload JSON para enriquecer vazamentos de crédito (reason/fix).
// value e lost_credit são calculados no Go; a LLM não deve alterá-los.
//
// LegalBase/AnnualValues/Effort/Risk/Priority (Etapa C/PR5) usam `json:"-"`
// deliberadamente: são dados 100% determinísticos do Go que só precisam
// sobreviver ao round-trip Go→LLM→Go (EnrichCreditLeaks copia o struct
// inteiro antes e depois da chamada) — não há motivo para gastar tokens
// mandando projeção de 8 anos para um prompt que só escreve reason/fix.
type CreditLeakEnrichmentItem struct {
	Description  string                            `json:"description"`
	Value        string                            `json:"value"`
	LostCredit   string                            `json:"lost_credit"`
	RegimeType   string                            `json:"regime_type,omitempty"`
	Reason       string                            `json:"reason,omitempty"`
	Fix          string                            `json:"fix,omitempty"`
	LegalBase    string                            `json:"-"`
	AnnualValues []CreditLeakAnnualValueEnrichment `json:"-"`
	Effort       string                            `json:"-"`
	Risk         string                            `json:"-"`
	Priority     string                            `json:"-"`
}

// CreditLeakAnnualValueEnrichment espelha tax.CreditLeakAnnualValue neste
// pacote (só para carregar o dado no round-trip — nunca serializado).
type CreditLeakAnnualValueEnrichment struct {
	Year       int
	LostCredit string
}
