package http

import "encoding/json"

// ExplanationRequest é o payload de POST /ai/explanations.
type ExplanationRequest struct {
	Question  string  `json:"question"`
	Threshold float64 `json:"threshold"`
	Limit     int     `json:"limit"`
}

// ExplanationResult representa um chunk retornado pelo RAG.
type ExplanationResult struct {
	ArticleID  string            `json:"article_id"`
	Content    string            `json:"content"`
	Similarity float64           `json:"similarity"`
	Type       string            `json:"type"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// ExplanationResponse é o payload de resposta de POST /ai/explanations.
type ExplanationResponse struct {
	Question string              `json:"question"`
	Results  []ExplanationResult `json:"results"`
}

// HealthResponse é o payload de resposta de GET /health.
type HealthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

// ErrorResponse é o envelope de erros retornado em qualquer falha.
type ErrorResponse struct {
	Error string `json:"error"`
	// Campos opcionais (PLG / quotas)
	Code  string `json:"code,omitempty"`
	Limit int    `json:"limit,omitempty"`
	Used  int    `json:"used,omitempty"`
	Plan  string `json:"plan,omitempty"`
	// RequestID correlaciona com logs do servidor (erros 5xx sanitizados).
	RequestID string `json:"request_id,omitempty"`
}

// PlgQuotaResponse é o payload de GET /plg/quota.
type PlgQuotaResponse struct {
	Plan               string `json:"plan"`
	SimulationsToday   int    `json:"simulations_today"`
	DailyLimit         int    `json:"daily_limit"`
	CompaniesCount     int    `json:"companies_count"`
	CompanyLimit       int    `json:"company_limit"`
	EnforcementEnabled bool   `json:"enforcement_enabled"`
}

// LawArticleResponse é o payload de GET /law/articles/{id} (texto integral remontado).
type LawArticleResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Source  string `json:"source"`
}

// LawPdfAnchorResponse é o payload de GET /law/articles/{id}/pdf-anchor (Pro/Premium).
type LawPdfAnchorResponse struct {
	PdfURL     string `json:"pdf_url"`
	Page       int    `json:"page"`
	PdfCoordY  string `json:"pdf_coord_y"`
	Convention string `json:"convention"`
	LeiVersion string `json:"lei_version,omitempty"`
	PrfFile    string `json:"prf_file,omitempty"`
}

// LawCorpusDocumentResponse é um documento do corpus normativo (GET /law/corpus).
type LawCorpusDocumentResponse struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Version     string `json:"version"`
	PublishedAt string `json:"published_at"`
	SourceURL   string `json:"source_url"`
	ChunkPrefix string `json:"chunk_prefix,omitempty"`
}

// LawCorpusChangelogEntryResponse é uma entrada factual do changelog do corpus
// (nunca release note inventada — ver internal/lawcorpus).
type LawCorpusChangelogEntryResponse struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Desc  string `json:"desc"`
}

// LawCorpusResponse é o payload de GET /law/corpus — reporta o que está
// REALMENTE ingerido em tax_law_chunks, nunca o que se deseja ter. Slices
// nunca nil (documents:[] / changelog:[] no corpus vazio, não null).
type LawCorpusResponse struct {
	Documents         []LawCorpusDocumentResponse       `json:"documents"`
	CurrentDocumentID string                            `json:"current_document_id"`
	Changelog         []LawCorpusChangelogEntryResponse `json:"changelog"`
}

// EngineValidationCaseResponse é o veredito de um ano na suíte cruzada contra
// a Calculadora RFB (GET /engine/validation, W7/B2.1-B2.3).
type EngineValidationCaseResponse struct {
	Year       int    `json:"year"`
	CBSTribIA  string `json:"cbs_tribia"`
	CBSRFB     string `json:"cbs_rfb"`
	IBSTribIA  string `json:"ibs_tribia"`
	IBSRFB     string `json:"ibs_rfb"`
	Divergente bool   `json:"divergente"`
}

// EngineValidationReferenceResponse identifica a calculadora usada como
// referência — ausente (omitempty, todos os campos vazios) quando Validated
// é false. Version é a versão da Calculadora RFB contra a qual a suíte rodou:
// a calculadora é beta e muda de versão, então o selo do dossiê diz "validado
// contra a versão X", nunca só "validado" (internal/enginevalidation exige a
// versão para afirmar Validated).
type EngineValidationReferenceResponse struct {
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	Version string `json:"version,omitempty"`
	RunAt   string `json:"run_at,omitempty"`
}

// EngineValidationResponse é o payload de GET /engine/validation — reporta o
// que a última execução da suíte cruzada (internal/tax/rfb_cross_test.go,
// build tag rfb) REALMENTE mostrou, nunca uma afirmação inventada
// (PRODUCT.md). Validated só é true com pelo menos 1 caso executado e zero
// divergências. Slices nunca nil.
type EngineValidationResponse struct {
	Validated      bool                              `json:"validated"`
	Reference      EngineValidationReferenceResponse `json:"reference"`
	Scope          []string                          `json:"scope"`
	OutOfScope     []string                          `json:"out_of_scope"`
	Tolerance      string                            `json:"tolerance_brl,omitempty"`
	Cases          []EngineValidationCaseResponse    `json:"cases"`
	CasesTotal     int                               `json:"cases_total"`
	CasesDivergent int                               `json:"cases_divergent"`
}

// --- Simulação tributária ---

// ServiceInput representa um serviço/receita de saída no payload da simulação.
// Amount e ISSRate são strings para evitar perda de precisão com float64 no JSON.
type ServiceInput struct {
	Description string `json:"description"`
	Amount      string `json:"amount"`                // ex: "10000.00"
	ISSRate     string `json:"iss_rate"`              // ex: "0.05" (5%)
	RegimeType  string `json:"regime_type,omitempty"` // "padrao" | "diferenciado_60" | "reduzido_zero"
}

// ExpenseInput representa uma despesa de entrada que pode gerar crédito.
type ExpenseInput struct {
	Description string `json:"description"`
	Amount      string `json:"amount"`
	IsEligible  bool   `json:"is_eligible"`
	RegimeType  string `json:"regime_type,omitempty"` // "padrao" | "diferenciado_60" | "reduzido_zero"
}

// SimulationRequest é o payload de POST /simulations.
type SimulationRequest struct {
	Year           int    `json:"year"`
	CompanyRegime  string `json:"company_regime,omitempty"` // incl. "imobiliario_venda" | "imobiliario_aluguel" | "prof_liberal" | "exportadora" | "entidade_imune"
	CompanyContext string `json:"company_context,omitempty"`
	// ImobiliarioRedutorAjusteBRL: valor em R$ abatido da receita total antes da alíquota (perfis imobiliários); vazio = env IMOBILIARIO_REDUTOR_* ou 0.
	ImobiliarioRedutorAjusteBRL string         `json:"imobiliario_redutor_ajuste_brl,omitempty"`
	Services                    []ServiceInput `json:"services"`
	Expenses                    []ExpenseInput `json:"expenses"`
}

// TaxComponentsResponse decompõe o bruto de um TaxBreakdownResponse por
// tributo — ver tax.TaxComponents. Preenchida no motor desde o W7/B2.1, nunca
// exposta até esta PR (W2/PR2, Etapa C, achado 3: docs/roadmap-execucao.md).
// A soma pode divergir de gross_tax em até R$ 0,01 por arredondamento
// independente — não é erro, é o mesmo fenômeno documentado em tax.TaxComponents.
// Fica zerada (não ausente) em regimes sem decomposição natural — MEI, Simples,
// imobiliário; ver o comentário em cada ramo de calculator.go.
type TaxComponentsResponse struct {
	Pis    string `json:"pis"`
	Cofins string `json:"cofins"`
	Iss    string `json:"iss"`
	Cbs    string `json:"cbs"`
	Ibs    string `json:"ibs"`
}

// CalculationStepInputResponse é um operando nomeado de um CalculationStepResponse.
type CalculationStepInputResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CalculationStepResponse é um passo da memória de cálculo — ver
// tax.CalculationStep (W2/PR1). Output/Inputs em precisão total, NÃO
// StringFixed(2): um passo intermediário propositalmente não-arredondado
// perderia a informação que existe para mostrar se fosse truncado a 2 casas.
type CalculationStepResponse struct {
	Item    string                         `json:"item,omitempty"`
	Label   string                         `json:"label"`
	Formula string                         `json:"formula"`
	Inputs  []CalculationStepInputResponse `json:"inputs,omitempty"`
	Output  string                         `json:"output"`
	Rounded bool                           `json:"rounded"`
}

// TaxBreakdownResponse detalha os componentes de um cenário tributário na resposta.
type TaxBreakdownResponse struct {
	GrossTax string `json:"gross_tax"`
	Credits  string `json:"credits"`
	NetTax   string `json:"net_tax"`
	// Components e Trace: ver os tipos acima (W2/PR2). Trace omitido (não
	// vazio) em registros antigos gravados antes desta PR — mesmo padrão de
	// enrichTransitionSeriesLegacy: campo ausente, seção não renderiza.
	Components TaxComponentsResponse     `json:"components"`
	Trace      []CalculationStepResponse `json:"trace,omitempty"`
}

// RuleBasisResponse é a proveniência de uma linha da tabela de transição —
// ver tax.RuleBasis. Auditada número a número na Onda 2/PR 7 (W1); esta PR
// (W2/PR2) é o que leva essa auditoria ao dossiê, em vez de ficar só em
// comentário de código.
type RuleBasisResponse struct {
	Kind string `json:"kind"` // lei_calendario | estimativa_oficial | premissa_tribia
	Note string `json:"note"`
}

// TransitionYearFactors expõe os insumos de RulesForYear(y) para auditoria (Excel / consultor PRO).
type TransitionYearFactors struct {
	Year                  int    `json:"year"`
	PisCofinsFactor       string `json:"pis_cofins_factor"` // 0–1 sobre alíquotas de referência PIS/COFINS
	CbsRate               string `json:"cbs_rate"`
	IbsRate               string `json:"ibs_rate"`
	CombinedProjectedRate string `json:"combined_projected_rate,omitempty"`
	IssMunicipalFactor    string `json:"iss_municipal_factor,omitempty"` // factor sobre ISS do input (transição municipal)
	IssModel              string `json:"iss_model,omitempty"`            // ex.: input_static | municipal_transition_lc68
	// Basis é a proveniência da linha usada neste ano — ver RuleBasisResponse.
	// Ausente (nil) em registro antigo gravado antes desta PR.
	Basis *RuleBasisResponse `json:"basis,omitempty"`
}

// TransitionSeriesPoint é um ponto do gráfico legado vs. CBS/IBS por ano de transição.
type TransitionSeriesPoint struct {
	Year        int    `json:"year"`
	OldTaxNet   string `json:"old_tax_net"`   // líquido regime atual (PIS/COFINS/ISS)
	NewTaxNet   string `json:"new_tax_net"`   // líquido projetado (CBS/IBS)
	TotalTaxNet string `json:"total_tax_net"` // old + new (empilhado no gráfico de carga combinada)
	// Current/Projected/Delta por ano permitem foco temporal sem novo POST (PRO).
	Current   TaxBreakdownResponse   `json:"current,omitempty"`
	Projected TaxBreakdownResponse   `json:"projected,omitempty"`
	Delta     string                 `json:"delta,omitempty"`
	DeltaPct  string                 `json:"delta_pct,omitempty"`
	Factors   *TransitionYearFactors `json:"factors,omitempty"`
}

// CreditLeakResponse descreve crédito não apropriado por despesa marcada inelegível (ilustrativo).
type CreditLeakResponse struct {
	Description string `json:"description"`
	Value       string `json:"value"`       // valor da despesa
	LostCredit  string `json:"lost_credit"` // valor × alíquota efetiva se fosse elegível
	Reason      string `json:"reason,omitempty"`
	Fix         string `json:"fix,omitempty"`
	RegimeType  string `json:"regime_type,omitempty"` // regime normalizado usado no cálculo
}

// SimulationResponse é o payload de resposta de POST /simulations.
// Valores monetários são strings (ex: "90.00") para preservar precisão decimal.
type SimulationResponse struct {
	Year          int    `json:"year"`
	CompanyRegime string `json:"company_regime,omitempty"`
	// Regime é o ramo de tax.Calculate que produziu este resultado — um dos
	// company_regime normalizados (ex.: "regular" para entrada vazia), NUNCA
	// o valor bruto do campo acima. Sem isto (W2/PR2, achado 2 da Etapa C),
	// quem lê o resultado não sabia por qual caminho de cálculo ele passou —
	// e os ramos usam fórmulas diferentes. Ausente em registro antigo.
	Regime           string                  `json:"regime,omitempty"`
	Current          TaxBreakdownResponse    `json:"current"`
	Projected        TaxBreakdownResponse    `json:"projected"`
	Delta            string                  `json:"delta"`
	DeltaPct         string                  `json:"delta_pct"`
	StrategyInsight  string                  `json:"strategy_insight,omitempty"`
	RevenueTotal     string                  `json:"revenue_total,omitempty"`     // soma dos serviços; para modo % no gráfico
	TransitionSeries []TransitionSeriesPoint `json:"transition_series,omitempty"` // 2026–2033
	CreditLeaks      []CreditLeakResponse    `json:"credit_leaks,omitempty"`
	// TransitionSeriesEnriched: true quando GET histórico reconstituiu fatores/breakdown (registo antigo).
	TransitionSeriesEnriched bool `json:"transition_series_enriched,omitempty"`
	// OverlapModel identifica o modo de convivência: duas simulações completas comparáveis por ano (sem blend prévio dos ramos).
	OverlapModel string `json:"overlap_model,omitempty"`
}

// --- Classification DTOs ---

// ClassificationRequest é o payload de entrada de POST /credit-classifications.
type ClassificationRequest struct {
	Description string `json:"description"`
	Context     string `json:"context,omitempty"`
}

// LegalPathResponse expõe a hierarquia normativa (do documento ingerido — ver GET /law/corpus) derivada da ingestão.
type LegalPathResponse struct {
	ArticleLabel string `json:"article_label,omitempty"`
	Paragraph    string `json:"paragraph,omitempty"`
	Inciso       string `json:"inciso,omitempty"`
	Alinea       string `json:"alinea,omitempty"`
	SpanNote     string `json:"span_note,omitempty"`
}

// EvidenceArticleResponse expõe um artigo da lei recuperado pelo RAG.
type EvidenceArticleResponse struct {
	ArticleID                 string             `json:"article_id"`
	Content                   string             `json:"content"`
	Similarity                float64            `json:"similarity"`
	Metadata                  map[string]string  `json:"metadata,omitempty"`
	LegalPath                 *LegalPathResponse `json:"legal_path,omitempty"`
	RelevantSnippets          []string           `json:"relevant_snippets,omitempty"`
	RelevantSnippetsTentative []string           `json:"relevant_snippets_tentative,omitempty"`
}

// MatchedSpanResponse âncora determinística no contexto da empresa (runas; início inclusivo, fim exclusivo).
type MatchedSpanResponse struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ClassificationResponse é o contrato de saída do classificador de créditos.
type ClassificationResponse struct {
	IsEligible    bool                      `json:"is_eligible"`
	Confidence    float64                   `json:"confidence"`
	Justification string                    `json:"justification"`
	LegalBase     string                    `json:"legal_base"`
	RiskLevel     string                    `json:"risk_level"`
	RegimeType    string                    `json:"regime_type"`
	Evidence      []EvidenceArticleResponse `json:"evidence"`
	MatchedSpan   *MatchedSpanResponse      `json:"matched_span,omitempty"`
}

// --- Batch Classification DTOs ---

// BatchExpenseInput representa uma despesa individual no payload de lote.
type BatchExpenseInput struct {
	ClientID    string `json:"client_id,omitempty"`
	Description string `json:"description"`
	Context     string `json:"context,omitempty"`
}

// BatchClassificationRequest é o payload de POST /credit-classifications/batch.
// MaxConcurrency controla quantas chamadas simultâneas à OpenAI são permitidas;
// default 5, máximo 10.
type BatchClassificationRequest struct {
	Expenses       []BatchExpenseInput `json:"expenses"`
	MaxConcurrency int                 `json:"max_concurrency,omitempty"`
}

// BatchClassificationItem é o resultado de uma despesa ou serviço individual no lote.
// Error fica vazio em caso de sucesso; preenchido se a classificação falhou
// para este item sem abortar os demais.
type BatchClassificationItem struct {
	ClientID      string  `json:"client_id,omitempty"`
	Description   string  `json:"description"`
	IsEligible    bool    `json:"is_eligible"`
	Confidence    float64 `json:"confidence"`
	Justification string  `json:"justification"`
	LegalBase     string  `json:"legal_base"`
	RiskLevel     string  `json:"risk_level"`
	// RegimeType expõe o regime tributário detectado pela IA — ver o
	// comentário de RegimeDiferenciado60 em internal/tax/rules.go para a
	// citação por categoria (não é um único artigo; auditado, Onda 2/W1).
	RegimeType  string                    `json:"regime_type"`
	Evidence    []EvidenceArticleResponse `json:"evidence"`
	MatchedSpan *MatchedSpanResponse      `json:"matched_span,omitempty"`
	Error       string                    `json:"error,omitempty"`
}

// BatchClassificationResponse é o contrato de saída de POST /credit-classifications/batch.
// Total = número de itens enviados; Processed = itens sem erro.
type BatchClassificationResponse struct {
	Total     int                       `json:"total"`
	Processed int                       `json:"processed"`
	Results   []BatchClassificationItem `json:"results"`
	// DiscoveredTags lista padrões inseridos neste request (base global de chips).
	DiscoveredTags []StrategyTagResponse `json:"discovered_tags,omitempty"`
}

// StrategyTagResponse é uma linha da tabela strategy_tags exposta à UI.
type StrategyTagResponse struct {
	Pattern     string `json:"pattern"`
	Label       string `json:"label"`
	Category    string `json:"category,omitempty"`
	ColorScheme string `json:"color_scheme"`
}

// StrategyTagsListResponse é o corpo de GET /strategy-tags.
type StrategyTagsListResponse struct {
	Tags []StrategyTagResponse `json:"tags"`
}

// --- Histórico de simulações (Supabase) ---

// ReportBrandSnapshot white-label (Premium) persistido no JSON do snapshot; servido no dossié público.
type ReportBrandSnapshot struct {
	LogoURL *string `json:"logo_url,omitempty"`
	OrgName *string `json:"org_name,omitempty"`
}

// ClassificationHistorySnapshot persiste evidências RAG e agregados para reidratar o dashboard como na 1.ª execução.
type ClassificationHistorySnapshot struct {
	SnapshotVersion        int                       `json:"snapshot_version,omitempty"`
	ServiceClassifications []BatchClassificationItem `json:"service_classifications,omitempty"`
	ExpenseClassifications []BatchClassificationItem `json:"expense_classifications,omitempty"`
	AiMetadata             json.RawMessage           `json:"ai_metadata,omitempty"`
	DiscoveredTags         []StrategyTagResponse     `json:"discovered_tags,omitempty"`
	ReportBrand            *ReportBrandSnapshot      `json:"report_brand,omitempty"`
}

// SimulationRecordCreateRequest é o corpo de POST /simulation-records.
type SimulationRecordCreateRequest struct {
	UserID          string                    `json:"user_id"`
	CompanyID       *string                   `json:"company_id,omitempty"`
	CompanyContext  string                    `json:"company_context"`
	CompanyRegime   string                    `json:"company_regime,omitempty"`
	Year            int                       `json:"year"`
	Simulation      SimulationResponse        `json:"simulation"`
	Services        []ServiceInput            `json:"services"`
	Expenses        []ExpenseInput            `json:"expenses"`
	Classifications []BatchClassificationItem `json:"classifications"`
	// ClassificationsSnapshot substitui o uso exclusivo de classifications para UI rica (evidências RAG).
	ClassificationsSnapshot *ClassificationHistorySnapshot `json:"classifications_snapshot,omitempty"`
}

// SimulationRecordCreateResponse retorna o id gravado.
type SimulationRecordCreateResponse struct {
	ID string `json:"id"`
}

// FormServiceDTO espelha o FormService do frontend (com id estável ao reidratar).
type FormServiceDTO struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Amount      string `json:"amount"`
	ISSRate     string `json:"iss_rate"`
}

// FormExpenseDTO espelha FormExpense do frontend.
type FormExpenseDTO struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Amount      string `json:"amount"`
}

// SimulationRecordDetailResponse é a resposta de GET /simulation-records/{id}.
type SimulationRecordDetailResponse struct {
	ID                      string                    `json:"id"`
	CreatedAt               string                    `json:"created_at"`
	Year                    int                       `json:"year"`
	CompanyID               *string                   `json:"company_id,omitempty"`
	CompanyContext          string                    `json:"company_context"`
	CompanyRegime           string                    `json:"company_regime,omitempty"`
	Simulation              SimulationResponse        `json:"simulation"`
	Services                []FormServiceDTO          `json:"services"`
	Expenses                []FormExpenseDTO          `json:"expenses"`
	Classifications         []BatchClassificationItem `json:"classifications"`
	ClassificationsSnapshot json.RawMessage           `json:"classifications_snapshot,omitempty"`
}

// --- Templates de Empresa ---

// CompanyCreateRequest é o payload de POST /companies.
type CompanyCreateRequest struct {
	Name            string          `json:"name"`
	TaxContext      string          `json:"tax_context"`
	DefaultServices json.RawMessage `json:"default_services"` // []ServiceInput como JSON
}

// CompanyResponse é o retorno de GET /companies e POST /companies.
type CompanyResponse struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	TaxContext      string          `json:"tax_context"`
	DefaultServices json.RawMessage `json:"default_services"`
	CreatedAt       string          `json:"created_at"`
}
