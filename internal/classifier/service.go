package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
	"github.com/jeanprocha/backend-engine-go/internal/rag"
)

const (
	ragThreshold = 0.35
	ragLimit     = 5
)

// anchorArticleIDs são os chunks que definem a regra geral de não-cumulatividade
// da LC 68/2024. Sempre injetados no topo do contexto da LLM, independente
// do resultado da busca semântica, garantindo que a regra geral nunca falte.
//
// IDs verificados diretamente no Supabase:
//   - Art. 28 não possui cabeçalho #### próprio; está na 2ª parte do chunk do Art. 26.
//     lc68_0018_art_26_p2: contém "O contribuinte sujeito ao regime regular do IBS e da
//     CBS poderá apropriar créditos desses tributos" — a regra substantiva de crédito.
//   - lc68_0019_art_29: Art. 29 — mecanismo de apropriação por destaque no documento fiscal.
var anchorArticleIDs = []string{
	"lc68_0018_art_26_p2", // Art. 28: regra geral — bens/serviços na atividade geram crédito
	"lc68_0019_art_29",    // Art. 29: como apropriar o crédito via documento fiscal
}

// expandQueryPrompt instrui a LLM a traduzir termos coloquiais em juridiquês.
// É um prompt leve, sem as restrições do systemPrompt blindado.
const expandQueryPrompt = `Você traduz descrições de despesas empresariais para termos jurídicos da Lei Complementar 68/2024 (Reforma Tributária CBS/IBS).
Retorne APENAS os termos técnicos separados por vírgula, sem nenhuma explicação.
Exemplo: "AWS" → "bens imateriais, licenciamento de software, serviços digitais, tecnologia da informação"
Exemplo: "streaming Netflix" → "bens imateriais, direitos, serviços digitais, plataformas de conteúdo"`

// Service orquestra RAG + LLM para classificar se uma despesa gera crédito de IBS/CBS.
type Service struct {
	rag *rag.Service
	llm *LLMClient
}

// NewService cria o classificador com as dependências injetadas.
func NewService(ragSvc *rag.Service, apiKey string) *Service {
	return &Service{
		rag: ragSvc,
		llm: newLLMClient(apiKey),
	}
}

// expandQuery transforma a descrição coloquial em termos jurídicos da LC 68/2024
// para melhorar a precisão da busca vetorial (resolve o "abismo semântico").
// Em caso de erro (timeout, quota), retorna a descrição original —
// graceful degradation garante que ClassifyExpense nunca falhe por causa desta etapa.
func (s *Service) expandQuery(ctx context.Context, description string) string {
	expanded, err := s.llm.Chat(ctx, expandQueryPrompt, description)
	if err != nil || strings.TrimSpace(expanded) == "" {
		log.Printf("classifier: expandQuery fallback para descrição original (err=%v)", err)
		return description
	}
	expanded = strings.TrimSpace(expanded)
	log.Printf("classifier: expandQuery %q -> %q", description, expanded)
	return expanded
}

// classificationLLMResponse é o schema esperado na saída da LLM.
type classificationLLMResponse struct {
	IsEligible    bool    `json:"is_eligible"`
	Confidence    float64 `json:"confidence"`
	Justification string  `json:"justification"`
	LegalBase     string  `json:"legal_base"`
	RiskLevel     string  `json:"risk_level"`
	// RegimeType classifica o regime tributário do item conforme Art. 131 LC 68/2024.
	RegimeType string `json:"regime_type"`
}

// ClassifyExpense classifica se uma despesa é elegível a crédito de IBS/CBS.
//
// Fluxo híbrido (expansão + âncoras):
//  1. expandQuery: LLM traduz termos coloquiais para juridiquês (graceful degradation).
//  2. RAG busca artigos por similaridade usando os termos expandidos.
//  3. GetByIDs busca os artigos âncora (Art. 29 e Art. 38) que definem a regra geral.
//  4. Merge: une âncoras + encontrados, deduplicando por ArticleID.
//  5. Se o contexto final for vazio: retorna inconclusivo sem chamar LLM.
//  6. LLM classifica com o contexto blindado e a categoria jurídica sugerida.
func (s *Service) ClassifyExpense(ctx context.Context, description, companyContext string) (ClassificationResult, error) {
	if strings.TrimSpace(description) == "" {
		return ClassificationResult{}, fmt.Errorf("classifier: description nao pode ser vazia")
	}

	// 1. Expande a query para termos jurídicos — melhora o recall da busca vetorial.
	expandedTerms := s.expandQuery(ctx, description)

	// 2. Busca semântica com os termos expandidos
	foundArticles, err := s.rag.Query(ctx, expandedTerms, ragThreshold, ragLimit)
	if err != nil {
		return ClassificationResult{}, fmt.Errorf("classifier: busca rag: %w", err)
	}

	// 3. Busca artigos âncora (regra geral de não-cumulatividade)
	anchorArticles, err := s.rag.GetByIDs(ctx, anchorArticleIDs)
	if err != nil {
		// Falha ao buscar âncoras não cancela o fluxo: segue sem elas.
		anchorArticles = nil
	}

	// 4. Merge: âncoras no topo (sempre visíveis para a LLM) + artigos específicos,
	// deduplicando para não enviar o mesmo artigo duas vezes.
	seen := make(map[string]bool, len(anchorArticles)+len(foundArticles))
	articles := make([]ingestion.SearchResult, 0, len(anchorArticles)+len(foundArticles))

	for _, a := range anchorArticles {
		if !seen[a.ArticleID] {
			seen[a.ArticleID] = true
			articles = append(articles, a)
		}
	}
	for _, a := range foundArticles {
		if !seen[a.ArticleID] {
			seen[a.ArticleID] = true
			articles = append(articles, a)
		}
	}

	// 5. Sem contexto algum: inconclusivo, sem chamar LLM
	if len(articles) == 0 {
		return ClassificationResult{
			IsEligible:    false,
			Confidence:    0,
			Justification: "Nenhum artigo da LC 68/2024 com similaridade suficiente foi encontrado para esta despesa.",
			LegalBase:     "",
			RiskLevel:     "alto",
			Evidence:      nil,
		}, nil
	}

	// 6. Monta mensagem de usuário com contexto jurídico e categoria sugerida.
	// expandedTerms serve de ponte semântica: a LLM vê "AWS (Categoria: licenciamento
	// de software, bens imateriais)" e pode aplicar o Art. 28 sem exigir que "AWS"
	// apareça literalmente no texto da lei.
	userMsg := buildUserMessage(description, expandedTerms, companyContext, articles)

	// 4. Chama a LLM
	rawJSON, err := s.llm.Chat(ctx, systemPrompt, userMsg)
	if err != nil {
		return ClassificationResult{}, fmt.Errorf("classifier: llm chat: %w", err)
	}

	// 5. Parse do JSON retornado pela LLM
	// Remove possíveis blocos de markdown que a LLM às vezes adiciona
	rawJSON = strings.TrimSpace(rawJSON)
	rawJSON = strings.TrimPrefix(rawJSON, "```json")
	rawJSON = strings.TrimPrefix(rawJSON, "```")
	rawJSON = strings.TrimSuffix(rawJSON, "```")
	rawJSON = strings.TrimSpace(rawJSON)

	var llmResp classificationLLMResponse
	if err := json.Unmarshal([]byte(rawJSON), &llmResp); err != nil {
		return ClassificationResult{}, fmt.Errorf("classifier: parse resposta llm (%q): %w", rawJSON, err)
	}

	// 6. Mapeia para ClassificationResult com evidências rastreáveis
	evidence := make([]EvidenceArticle, 0, len(articles))
	for _, a := range articles {
		evidence = append(evidence, EvidenceArticle{
			ArticleID:  a.ArticleID,
			Content:    a.Content,
			Similarity: a.Similarity,
		})
	}

	// Normaliza regime_type: garante que valor vazio ou desconhecido vira "padrao".
	regimeType := llmResp.RegimeType
	switch regimeType {
	case "diferenciado_60", "reduzido_zero":
		// válidos — mantém
	default:
		regimeType = "padrao"
	}

	return ClassificationResult{
		IsEligible:    llmResp.IsEligible,
		Confidence:    llmResp.Confidence,
		Justification: llmResp.Justification,
		LegalBase:     llmResp.LegalBase,
		RiskLevel:     llmResp.RiskLevel,
		RegimeType:    regimeType,
		Evidence:      evidence,
	}, nil
}

// BatchItem é a unidade de entrada de um lote de classificações.
type BatchItem struct {
	Description    string
	CompanyContext string
}

// BatchResult é o resultado de um item do lote, preservando a descrição original
// para rastreabilidade e carregando o erro individual sem abortar o lote inteiro.
type BatchResult struct {
	Description string
	ClassificationResult
	Err string // não vazio se ClassifyExpense falhou para este item
}

// ClassifyBatch classifica múltiplas despesas em paralelo com controle de concorrência.
// O semáforo (canal com buffer de tamanho maxConcurrency) garante que no máximo
// maxConcurrency chamadas simultâneas à OpenAI ocorram a qualquer momento,
// respeitando rate limits e recursos do servidor.
// A ordem do slice de saída corresponde exatamente à ordem do slice de entrada.
func (s *Service) ClassifyBatch(ctx context.Context, items []BatchItem, maxConcurrency int) []BatchResult {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}

	results := make([]BatchResult, len(items))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		go func(idx int, it BatchItem) {
			defer wg.Done()

			// Ocupa um slot no semáforo — bloqueia se maxConcurrency estiver cheio.
			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := s.ClassifyExpense(ctx, it.Description, it.CompanyContext)
			if err != nil {
				results[idx] = BatchResult{
					Description: it.Description,
					Err:         err.Error(),
				}
				return
			}
			results[idx] = BatchResult{
				Description:          it.Description,
				ClassificationResult: res,
			}
		}(i, item)
	}

	wg.Wait()
	return results
}
