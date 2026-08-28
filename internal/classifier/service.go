package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
	"github.com/jeanprocha/backend-engine-go/internal/rag"
)

const ragLimit = 5

// ragThreshold é o limiar mínimo de similaridade para chunks RAG (recall vs ruído).
// Valor por defeito 0,35; sobrescrever com CLASSIFIER_RAG_THRESHOLD (0–1).
var ragThreshold = 0.35

func init() {
	s := strings.TrimSpace(os.Getenv("CLASSIFIER_RAG_THRESHOLD"))
	if s == "" {
		return
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || f > 1 {
		slog.Warn("classifier_rag_threshold_invalid", "value", s, "fallback", ragThreshold)
		return
	}
	ragThreshold = f
}

// anchorArticleIDs são os chunks que definem a regra geral de não-cumulatividade
// do documento default. Sempre injetados no topo do contexto da LLM, independente
// do resultado da busca semântica, garantindo que a regra geral nunca falte.
//
// IDs verificados contra produção (GET /law/articles/{id} = 200) na virada da
// Onda 2/PR 6, documento default = LC 214/2025 (ver ingestion.DefaultDocumentProfile):
//   - lc214_0048_art_47_p1: Art. 47, § 1º — "o contribuinte sujeito ao regime regular
//     poderá apropriar créditos do IBS e da CBS…" — a regra substantiva de crédito,
//     equivalente direto do antigo lc68_0018_art_26_p2.
//   - lc214_0050_art_49: Art. 49 — operações imunes, isentas, alíquota zero, diferimento
//     e suspensão NÃO permitem crédito. Escolha deliberada (difere do padrão anterior,
//     que era o mecanismo de apropriação, Art. 29): para um classificador que decide
//     elegibilidade, a regra NEGATIVA é mais útil que a procedimental.
//
// Overridável via CLASSIFIER_ANCHOR_ARTICLE_IDS (CSV) — obrigatório trocar
// sempre que o documento/prefixo de article_id mudar (ver ingestion.
// DocumentProfile), senão estes IDs deixam de casar com qualquer linha da
// tabela e GetByIDs devolve zero resultados (silenciosamente — ver o warn
// abaixo em ClassifyExpense, que é o único sinal disso em produção).
var anchorArticleIDs = []string{
	"lc214_0048_art_47_p1", // Art. 47, § 1º: regra geral — bens/serviços na atividade geram crédito
	"lc214_0050_art_49",    // Art. 49: operações imunes/isentas/alíquota zero não geram crédito
}

// parseAnchorArticleIDs é puro (testável) — a leitura de env fica em init().
func parseAnchorArticleIDs(raw string) []string {
	var ids []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			ids = append(ids, p)
		}
	}
	return ids
}

func init() {
	raw := strings.TrimSpace(os.Getenv("CLASSIFIER_ANCHOR_ARTICLE_IDS"))
	if raw == "" {
		return
	}
	if ids := parseAnchorArticleIDs(raw); len(ids) > 0 {
		anchorArticleIDs = ids
		return
	}
	slog.Warn("classifier_anchor_article_ids_invalid", "value", raw, "fallback", anchorArticleIDs)
}

// missingAnchorIDs devolve os IDs pedidos que não vieram em found — usado só
// para observabilidade (o fluxo de classificação segue sem eles).
func missingAnchorIDs(requested []string, found []ingestion.SearchResult) []string {
	foundSet := make(map[string]bool, len(found))
	for _, f := range found {
		foundSet[f.ArticleID] = true
	}
	var missing []string
	for _, id := range requested {
		if !foundSet[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

// defaultLawLabel é o rótulo do documento default (o que está no banco hoje —
// ver ingestion.DefaultDocumentProfile). Usado nos prompts que rodam ANTES de
// qualquer chunk ser recuperado (expandQuery, EnrichCreditLeaks) ou quando a
// busca RAG não retorna nada — nesses casos não há artigo do qual derivar o
// rótulo de verdade (ver lawLabelFromArticles).
func defaultLawLabel() string {
	return ingestion.DefaultDocumentProfile().SourceLabel
}

// lawLabelFromArticles deriva o rótulo do documento a partir do primeiro chunk
// com metadata.source preenchido — é o texto que a LLM efetivamente recebe
// nesta chamada (dois documentos podem coexistir no corpus por prefixo, ver
// W1/Onda 2). Fallback: defaultLawLabel() quando nenhum artigo tiver esse
// metadado (chunks antigos sem "source").
func lawLabelFromArticles(articles []ingestion.SearchResult) string {
	for _, a := range articles {
		if v := strings.TrimSpace(a.Metadata["source"]); v != "" {
			return v
		}
	}
	return defaultLawLabel()
}

// ragMatchLog estrutura estável para JSON nos logs (evidência RAG).
type ragMatchLog struct {
	ArticleID  string  `json:"article_id"`
	Similarity float64 `json:"similarity"`
	Source     string  `json:"source"` // "anchor" ou "semantic"
	LegalPath  string  `json:"legal_path,omitempty"`
}

func redactForLog(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

func isAnchorArticleID(id string) bool {
	for _, a := range anchorArticleIDs {
		if a == id {
			return true
		}
	}
	return false
}

func cloneMeta(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// expandQueryPromptTemplate instrui a LLM a traduzir termos coloquiais em
// juridiquês. É um prompt leve, sem as restrições do systemPrompt blindado.
// Roda ANTES da busca RAG — sem chunks ainda, então buildExpandQueryPrompt
// sempre recebe defaultLawLabel(), nunca um rótulo derivado de artigo.
const expandQueryPromptTemplate = `Você traduz descrições de despesas empresariais para termos jurídicos da {{LAW}} (Reforma Tributária CBS/IBS).
Retorne APENAS os termos técnicos separados por vírgula, sem nenhuma explicação.
Exemplo: "AWS" → "bens imateriais, licenciamento de software, serviços digitais, tecnologia da informação"
Exemplo: "streaming Netflix" → "bens imateriais, direitos, serviços digitais, plataformas de conteúdo"`

func buildExpandQueryPrompt(lawLabel string) string {
	return strings.ReplaceAll(expandQueryPromptTemplate, "{{LAW}}", lawLabel)
}

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

// expandQuery transforma a descrição coloquial em termos jurídicos da lei
// (defaultLawLabel) para melhorar a precisão da busca vetorial (resolve o
// "abismo semântico").
// Em caso de erro (timeout, quota), retorna a descrição original —
// graceful degradation garante que ClassifyExpense nunca falhe por causa desta etapa.
func (s *Service) expandQuery(ctx context.Context, description string) (string, TokenUsage) {
	cr, err := s.llm.Chat(ctx, buildExpandQueryPrompt(defaultLawLabel()), description)
	if err != nil {
		slog.Error("openai_chat_failed",
			"stage", "expand",
			"err", err.Error(),
			"description_redacted", redactForLog(description, 64),
			"expand_tokens", cr.Usage.TotalTokens,
		)
		return description, cr.Usage
	}
	if strings.TrimSpace(cr.Content) == "" {
		slog.Warn("classifier_expand_query_fallback",
			"reason", "empty_content",
			"description_redacted", redactForLog(description, 64),
			"expand_tokens", cr.Usage.TotalTokens,
		)
		return description, cr.Usage
	}
	expanded := strings.TrimSpace(cr.Content)
	slog.Debug("classifier_expand_query_ok",
		"description_redacted", redactForLog(description, 64),
		"expanded_redacted", redactForLog(expanded, 120),
		"expand_tokens", cr.Usage.TotalTokens,
	)
	return expanded, cr.Usage
}

// classificationLLMResponse é o schema esperado na saída da LLM.
type classificationLLMResponse struct {
	IsEligible    bool    `json:"is_eligible"`
	Confidence    float64 `json:"confidence"`
	Justification string  `json:"justification"`
	LegalBase     string  `json:"legal_base"`
	// PrimaryEvidenceIndex é 1-based: corresponde a [N] no contexto jurídico enviado ao modelo.
	// O servidor reconstrói a citação exacta a partir dos metadados do chunk (Opção A).
	PrimaryEvidenceIndex *int   `json:"primary_evidence_index,omitempty"`
	RiskLevel            string `json:"risk_level"`
	// RegimeType classifica o regime tributário do item conforme Art. 131 da lei (ver systemPromptTemplate).
	RegimeType string `json:"regime_type"`
	// SuggestedTags opcional; omitido na maior parte das respostas.
	SuggestedTags []struct {
		Pattern     string `json:"pattern"`
		Label       string `json:"label"`
		Category    string `json:"category"`
		ColorScheme string `json:"color_scheme"`
	} `json:"suggested_tags"`
	// MatchedSpan opcional: índices de runas no CONTEXTO DA EMPRESA (início inclusivo, fim exclusivo).
	MatchedSpan *struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"matched_span,omitempty"`
	// EvidenceHighlights: âncoras literais no texto de cada bloco [N] para realce na UI (validadas no Go).
	EvidenceHighlights []EvidenceHighlightEntry `json:"evidence_highlights,omitempty"`
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
//
// clientID é opcional (ex.: id do cliente no lote); usado apenas em logs estruturados.
func (s *Service) ClassifyExpense(ctx context.Context, description, companyContext, clientID string) (ClassificationResult, error) {
	start := time.Now()
	if strings.TrimSpace(description) == "" {
		return ClassificationResult{}, fmt.Errorf("classifier: description nao pode ser vazia")
	}

	// 1. Expande a query para termos jurídicos — melhora o recall da busca vetorial.
	expandedTerms, expandUsage := s.expandQuery(ctx, description)

	// 2. Busca semântica com os termos expandidos
	foundArticles, err := s.rag.Query(ctx, expandedTerms, ragThreshold, ragLimit)
	if err != nil {
		return ClassificationResult{}, fmt.Errorf("classifier: busca rag: %w", err)
	}

	// 3. Busca artigos âncora (regra geral de não-cumulatividade)
	anchorArticles, err := s.rag.GetByIDs(ctx, anchorArticleIDs)
	if err != nil {
		// Falha ao buscar âncoras não cancela o fluxo: segue sem elas — mas
		// registra, porque isso degrada silenciosamente a qualidade da
		// classificação (a regra geral de crédito deixa de estar sempre
		// presente no contexto da LLM).
		slog.Warn("classifier_anchor_fetch_failed", "err", err.Error(), "anchor_ids", anchorArticleIDs)
		anchorArticles = nil
	} else if len(anchorArticles) < len(anchorArticleIDs) {
		slog.Warn("classifier_anchor_articles_missing",
			"requested", len(anchorArticleIDs),
			"found", len(anchorArticles),
			"missing", missingAnchorIDs(anchorArticleIDs, anchorArticles),
		)
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

	// 5. Sem contexto algum: inconclusivo, sem chamar LLM (não confundir com falha de parse JSON).
	if len(articles) == 0 {
		slog.Info("credit_classification_inconclusive",
			"latency_ms", time.Since(start).Milliseconds(),
			"expand_tokens", expandUsage.TotalTokens,
			"reason", "no_articles_after_rag",
			"client_id", clientID,
			"description_redacted", redactForLog(description, 64),
		)
		return ClassificationResult{
			IsEligible: false,
			Confidence: 0.15,
			Justification: fmt.Sprintf("Nenhum trecho da %s atingiu o limiar mínimo de similaridade na busca "+
				"para esta descrição; a classificação por modelo de linguagem não foi aplicada.", defaultLawLabel()),
			LegalBase:     "",
			RiskLevel:     "alto",
			Evidence:      nil,
			SuggestedTags: nil,
			MatchedSpan:   nil,
		}, nil
	}

	// 6. Monta mensagem de usuário com contexto jurídico e categoria sugerida.
	// expandedTerms serve de ponte semântica: a LLM vê "AWS (Categoria: licenciamento
	// de software, bens imateriais)" e pode aplicar o Art. 28 sem exigir que "AWS"
	// apareça literalmente no texto da lei.
	ctxAug := augmentProfissionalLiberalProfile(augmentImobiliarioProfile(augmentExportadoraProfile(augmentAliquotaZeroProfile(augmentEntidadeImuneProfile(augmentCompanyContextForSectorClassifier(companyContext))))))
	userMsg := buildUserMessage(description, expandedTerms, ctxAug, articles)
	if lowSectorAlignmentWarning(companyContext, description) {
		userMsg += "\n\nAVISO DE CONSISTÊNCIA: possível desalinhamento entre o setor descrito no CONTEXTO DA EMPRESA e a natureza da despesa. " +
			"Não presuma elegibilidade por analogia com outros setores; fundamente-se nos artigos recuperados e nas regras de isolamento de setor (17)."
	}

	// 7. Chama a LLM de classificação
	classifyCR, err := s.llm.Chat(ctx, buildSystemPrompt(lawLabelFromArticles(articles)), userMsg)
	if err != nil {
		slog.Error("openai_chat_failed",
			"stage", "classify",
			"err", err.Error(),
			"client_id", clientID,
			"description_redacted", redactForLog(description, 64),
		)
		return ClassificationResult{}, fmt.Errorf("classifier: llm chat: %w", err)
	}
	rawJSON := classifyCR.Content

	// 5. Parse do JSON retornado pela LLM
	// Remove possíveis blocos de markdown que a LLM às vezes adiciona
	rawJSON = strings.TrimSpace(rawJSON)
	rawJSON = strings.TrimPrefix(rawJSON, "```json")
	rawJSON = strings.TrimPrefix(rawJSON, "```")
	rawJSON = strings.TrimSuffix(rawJSON, "```")
	rawJSON = strings.TrimSpace(rawJSON)

	var llmResp classificationLLMResponse
	parseErr := json.Unmarshal([]byte(rawJSON), &llmResp)
	if parseErr != nil {
		if repaired := extractJSONObject(rawJSON); repaired != "" {
			parseErr = json.Unmarshal([]byte(repaired), &llmResp)
		}
	}
	if parseErr != nil {
		slog.Error("credit_classification_parse_failed",
			"err", parseErr.Error(),
			"client_id", clientID,
			"description_redacted", redactForLog(description, 64),
		)
		return ClassificationResult{}, fmt.Errorf("classifier: parse resposta llm (%q): %w", rawJSON, parseErr)
	}

	matchLogs := make([]ragMatchLog, 0, len(articles))
	for _, a := range articles {
		src := "semantic"
		if isAnchorArticleID(a.ArticleID) {
			src = "anchor"
		}
		matchLogs = append(matchLogs, ragMatchLog{
			ArticleID:  a.ArticleID,
			Similarity: a.Similarity,
			Source:     src,
			LegalPath:  ingestion.FormatLegalCitation(a.Metadata),
		})
	}

	slog.Info("credit_classification_completed",
		"latency_ms", time.Since(start).Milliseconds(),
		"expand_tokens", expandUsage.TotalTokens,
		"classify_tokens", classifyCR.Usage.TotalTokens,
		"total_llm_tokens", expandUsage.TotalTokens+classifyCR.Usage.TotalTokens,
		"rag_matches", matchLogs,
		"top_article_id", articles[0].ArticleID,
		"top_similarity", articles[0].Similarity,
		"client_id", clientID,
		"description_redacted", redactForLog(description, 64),
	)

	// 8. Mapeia para ClassificationResult com evidências rastreáveis
	evidence := make([]EvidenceArticle, 0, len(articles))
	for _, a := range articles {
		evidence = append(evidence, EvidenceArticle{
			ArticleID:  a.ArticleID,
			Content:    a.Content,
			Similarity: a.Similarity,
			Metadata:   cloneMeta(a.Metadata),
		})
	}

	applyEvidenceHighlights(&llmResp, evidence)

	// Normaliza regime_type: garante que valor vazio ou desconhecido vira "padrao".
	regimeType := llmResp.RegimeType
	switch regimeType {
	case "diferenciado_60", "reduzido_zero":
		// válidos — mantém
	default:
		regimeType = "padrao"
	}

	suggested := make([]SuggestedTag, 0, len(llmResp.SuggestedTags))
	for _, t := range llmResp.SuggestedTags {
		if strings.TrimSpace(t.Pattern) == "" || strings.TrimSpace(t.Label) == "" {
			continue
		}
		suggested = append(suggested, SuggestedTag{
			Pattern:     strings.TrimSpace(t.Pattern),
			Label:       strings.TrimSpace(t.Label),
			Category:    strings.TrimSpace(t.Category),
			ColorScheme: strings.TrimSpace(t.ColorScheme),
		})
		if len(suggested) >= 3 {
			break
		}
	}

	var matched *MatchedSpan
	if llmResp.MatchedSpan != nil {
		matched = NormalizeMatchedSpan(companyContext, llmResp.MatchedSpan.Start, llmResp.MatchedSpan.End)
	}

	legalBase, riskLevel := applyDeterministicCitation(&llmResp, evidence)

	return ClassificationResult{
		IsEligible:    llmResp.IsEligible,
		Confidence:    llmResp.Confidence,
		Justification: stripRedundantLegalEcho(legalBase, llmResp.Justification),
		LegalBase:     legalBase,
		RiskLevel:     riskLevel,
		RegimeType:    regimeType,
		Evidence:      evidence,
		SuggestedTags: suggested,
		MatchedSpan:   matched,
	}, nil
}

// stripRedundantLegalEcho evita justificativa repetir a mesma citação já em legal_base.
func stripRedundantLegalEcho(legalBase, justification string) string {
	legal := strings.TrimSpace(legalBase)
	just := strings.TrimSpace(justification)
	if legal == "" || just == "" {
		return just
	}
	lowL, lowJ := strings.ToLower(legal), strings.ToLower(just)
	if strings.HasPrefix(lowJ, lowL) {
		rest := strings.TrimSpace(just[len(legal):])
		rest = strings.TrimLeft(rest, " ,.;—–-")
		if rest != "" {
			return rest
		}
	}
	tail := ", conforme " + legal
	if len(just) > len(tail) && strings.HasSuffix(lowJ, strings.ToLower(tail)) {
		return strings.TrimSpace(just[:len(just)-len(tail)])
	}
	tail2 := " conforme " + legal
	if len(just) > len(tail2) && strings.HasSuffix(lowJ, strings.ToLower(tail2)) {
		return strings.TrimSpace(just[:len(just)-len(tail2)])
	}
	return just
}

// BatchItem é a unidade de entrada de um lote de classificações.
type BatchItem struct {
	ClientID       string
	Description    string
	CompanyContext string
}

// BatchResult é o resultado de um item do lote, preservando a descrição original
// para rastreabilidade e carregando o erro individual sem abortar o lote inteiro.
type BatchResult struct {
	ClientID    string
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

			res, err := s.ClassifyExpense(ctx, it.Description, it.CompanyContext, it.ClientID)
			if err != nil {
				results[idx] = BatchResult{
					ClientID:    it.ClientID,
					Description: it.Description,
					Err:         err.Error(),
				}
				return
			}
			results[idx] = BatchResult{
				ClientID:             it.ClientID,
				Description:          it.Description,
				ClassificationResult: res,
			}
		}(i, item)
	}

	wg.Wait()
	return results
}

// augmentCompanyContextForSectorClassifier reforça instruções quando o simulador sinaliza
// perfil de regime diferenciado (prefixo do frontend ou texto equivalente).
func augmentCompanyContextForSectorClassifier(companyContext string) string {
	c := strings.ToLower(companyContext)
	if !strings.Contains(c, "regime diferenciado") && !strings.Contains(c, "perfil simulador") {
		return companyContext
	}
	// Sem número de anexo (II, III): a numeração pode ter mudado entre o PLP
	// e a lei sancionada — validar é trabalho da auditoria da Onda 2, não
	// desta refatoração. "os anexos da lei" continua correto mesmo se mudar.
	block := fmt.Sprintf("\n\n[Instrução setorial — perfil regime diferenciado] Trate o contexto como saúde, educação ou cultura quando coerente com a descrição: priorize elegibilidade a crédito em insumos hospitalares, exames e apoio diagnóstico, materiais e serviços educacionais e equipamentos culturais. Verifique com rigor insumos específicos desses setores; quando os trechos recuperados acima citarem listas ou anexos da %s, aplique-os. Para itens claramente de atividade-fim, o risco de glosa tende a ser menor — sempre com base exclusiva nesses trechos; não invente normas fora do contexto recuperado.", defaultLawLabel())
	return companyContext + block
}

// augmentEntidadeImuneProfile reforça instruções quando o simulador sinaliza perfil entidade_imune (slug ou prefixo do frontend).
func augmentEntidadeImuneProfile(companyContext string) string {
	c := strings.ToLower(companyContext)
	if !strings.Contains(c, "entidade_imune") &&
		!strings.Contains(c, "entidade imune") &&
		!strings.Contains(c, "isfl") &&
		!strings.Contains(c, "sem fins lucrativos") {
		return companyContext
	}
	const block = "\n\n[Instrução — perfil entidade_imune] No modelo TribIA esta entidade não apropria créditos de IBS/CBS nas compras. Seja conservador: tendência a is_eligible false salvo quando trechos recuperados sustentarem exceção clara. Fundamente qualquer conclusão exclusivamente nos artigos fornecidos acima."
	return companyContext + block
}

// augmentAliquotaZeroProfile reforça instruções quando o simulador sinaliza perfil de cesta básica /
// alíquota zero na saída (prefixo do frontend ou texto equivalente).
func augmentAliquotaZeroProfile(companyContext string) string {
	c := strings.ToLower(companyContext)
	if !strings.Contains(c, "aliquota_zero") &&
		!strings.Contains(c, "cesta básica nacional") &&
		!strings.Contains(c, "cesta basica nacional") &&
		!strings.Contains(c, "alíquota zero") &&
		!strings.Contains(c, "aliquota zero") {
		return companyContext
	}
	// Sem número de anexo (I) — mesma cautela do bloco setorial acima.
	block := fmt.Sprintf("\n\n[Instrução — perfil cesta básica / alíquota zero na saída] Identifique se o item se enquadra na Cesta Básica Nacional conforme os anexos da %s presentes nos trechos recuperados acima, com base exclusiva neles. Itens de luxo ou fora das listas do contexto (ex.: produtos gourmet não cobertos pelo texto) devem receber regime_type \"padrao\" e não \"reduzido_zero\". Quando o contexto recuperado não sustentar alíquota zero, não a presuma.", defaultLawLabel())
	return companyContext + block
}

// augmentExportadoraProfile reforça instruções quando o simulador sinaliza perfil exportadora (slug ou prefixo do frontend).
func augmentExportadoraProfile(companyContext string) string {
	c := strings.ToLower(companyContext)
	if !strings.Contains(c, "exportadora") &&
		!strings.Contains(c, "mercado externo") &&
		!strings.Contains(c, "fretes internacionais") {
		return companyContext
	}
	block := fmt.Sprintf("\n\n[Instrução — perfil exportadora] Avalie elegibilidade a crédito IBS/CBS para fretes internacionais, armazenagem portuária ou logística, serviços de despachante aduaneiro e insumos claramente ligados à cadeia de exportação apenas quando sustentados pelos trechos recuperados acima da %s. Não presuma regime especial ou isenção sem âncora no texto fornecido.", defaultLawLabel())
	return companyContext + block
}

// augmentImobiliarioProfile reforça instruções para incorporação, venda ou locação (prefixo do frontend).
func augmentImobiliarioProfile(companyContext string) string {
	c := strings.ToLower(companyContext)
	if !strings.Contains(c, "imobiliario_venda") &&
		!strings.Contains(c, "imobiliario_aluguel") &&
		!strings.Contains(c, "setor imobili") {
		return companyContext
	}
	const block = "\n\n[Instrução — perfil imobiliário] Avalie elegibilidade a crédito IBS/CBS para materiais de construção (cimento, aço, argamassa etc.), equipamentos e serviços de empreiteira ou subempreitada claramente ligados à atividade de incorporação, construção ou locação de imóveis, com base exclusiva nos trechos recuperados acima. Não prometa crédito sem âncora no texto da lei fornecido."
	return companyContext + block
}

// augmentProfissionalLiberalProfile reforça instruções para perfil prof_liberal (prefixo do frontend ou slug).
func augmentProfissionalLiberalProfile(companyContext string) string {
	c := strings.ToLower(companyContext)
	if !strings.Contains(c, "prof_liberal") &&
		!strings.Contains(c, "profissões regulamentadas") &&
		!strings.Contains(c, "profissoes regulamentadas") &&
		!strings.Contains(c, "sociedade ou escritório de profissões") {
		return companyContext
	}
	const block = "\n\n[Instrução — perfil prof_liberal] Avalie com atenção softwares de gestão e ERP jurídico, tokens e certificados de assinatura digital, assinaturas de bases de dados e locação de sala ou escritório quando coerentes com a atividade-fim profissional; créditos IBS/CBS dependem dos trechos recuperados acima (Art. 28 e demais normas citadas). Não invente benefícios sem âncora no texto fornecido."
	return companyContext + block
}

// extractJSONObject isola o PRIMEIRO objeto JSON completo da resposta da LLM,
// descartando o que vier depois (texto solto, cerca de markdown, ou uma chave
// de fechamento a mais).
//
// A versão anterior ia do primeiro "{" até o ÚLTIMO "}" — o que não isola
// objeto nenhum quando o lixo é justamente um "}" extra no fim: a fatia saía
// idêntica à entrada e o reparo virava no-op. Medido em produção (Onda 2/PR 6):
// a LLM emitia `...,"suggested_tags":[]}}` em cerca de 40% das chamadas, e
// `json.Unmarshal` respondia `invalid character '}' after top-level value`.
// A classificação inteira falhava — is_eligible false, confidence 0, sem
// evidências —, e o dossiê perdia o item.
//
// json.Decoder lê exatamente UM valor e para; o resto do buffer é ignorado.
// Resolve o "}" a mais e qualquer outro sufixo, sem precisar adivinhar onde o
// objeto termina.
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	var raw json.RawMessage
	if err := json.NewDecoder(strings.NewReader(s[start:])).Decode(&raw); err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
