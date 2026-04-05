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
	ArticleID  string  `json:"article_id"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"`
	Type       string  `json:"type"`
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
}

// --- Simulação tributária ---

// ServiceInput representa um serviço/receita de saída no payload da simulação.
// Amount e ISSRate são strings para evitar perda de precisão com float64 no JSON.
type ServiceInput struct {
	Description string `json:"description"`
	Amount      string `json:"amount"`              // ex: "10000.00"
	ISSRate     string `json:"iss_rate"`            // ex: "0.05" (5%)
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
	Year     int            `json:"year"`
	Services []ServiceInput `json:"services"`
	Expenses []ExpenseInput `json:"expenses"`
}

// TaxBreakdownResponse detalha os componentes de um cenário tributário na resposta.
type TaxBreakdownResponse struct {
	GrossTax string `json:"gross_tax"`
	Credits  string `json:"credits"`
	NetTax   string `json:"net_tax"`
}

// SimulationResponse é o payload de resposta de POST /simulations.
// Valores monetários são strings (ex: "90.00") para preservar precisão decimal.
type SimulationResponse struct {
	Year      int                  `json:"year"`
	Current   TaxBreakdownResponse `json:"current"`
	Projected TaxBreakdownResponse `json:"projected"`
	Delta     string               `json:"delta"`
	DeltaPct  string               `json:"delta_pct"`
}

// --- Classification DTOs ---

// ClassificationRequest é o payload de entrada de POST /credit-classifications.
type ClassificationRequest struct {
	Description string `json:"description"`
	Context     string `json:"context,omitempty"`
}

// EvidenceArticleResponse expõe um artigo da lei recuperado pelo RAG.
type EvidenceArticleResponse struct {
	ArticleID  string  `json:"article_id"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"`
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
}

// --- Batch Classification DTOs ---

// BatchExpenseInput representa uma despesa individual no payload de lote.
type BatchExpenseInput struct {
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
	Description   string                    `json:"description"`
	IsEligible    bool                      `json:"is_eligible"`
	Confidence    float64                   `json:"confidence"`
	Justification string                    `json:"justification"`
	LegalBase     string                    `json:"legal_base"`
	RiskLevel     string                    `json:"risk_level"`
	// RegimeType expõe o regime tributário detectado pela IA (Art. 131 LC 68/2024).
	RegimeType    string                    `json:"regime_type"`
	Evidence      []EvidenceArticleResponse `json:"evidence"`
	Error         string                    `json:"error,omitempty"`
}

// BatchClassificationResponse é o contrato de saída de POST /credit-classifications/batch.
// Total = número de itens enviados; Processed = itens sem erro.
type BatchClassificationResponse struct {
	Total     int                       `json:"total"`
	Processed int                       `json:"processed"`
	Results   []BatchClassificationItem `json:"results"`
}

// --- Histórico de simulações (Supabase) ---

// SimulationRecordCreateRequest é o corpo de POST /simulation-records.
type SimulationRecordCreateRequest struct {
	UserID          string                    `json:"user_id"`
	OrganizationID  *string                   `json:"organization_id,omitempty"`
	CompanyContext  string                    `json:"company_context"`
	Year            int                       `json:"year"`
	Simulation      SimulationResponse        `json:"simulation"`
	Services        []ServiceInput            `json:"services"`
	Expenses        []ExpenseInput            `json:"expenses"`
	Classifications []BatchClassificationItem `json:"classifications"`
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
	ID              string                    `json:"id"`
	CreatedAt       string                    `json:"created_at"`
	Year            int                       `json:"year"`
	CompanyContext  string                    `json:"company_context"`
	Simulation      SimulationResponse        `json:"simulation"`
	Services        []FormServiceDTO          `json:"services"`
	Expenses        []FormExpenseDTO          `json:"expenses"`
	Classifications []BatchClassificationItem `json:"classifications"`
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
