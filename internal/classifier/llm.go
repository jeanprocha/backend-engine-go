package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	openAIChatURL = "https://api.openai.com/v1/chat/completions"
	chatModel     = "gpt-4o-mini"
	// chatTemperature fixo em 0: saída determinística para o mesmo input (classificação + expandQuery).
	chatTemperature = 0.0
	chatTopP        = 1.0
	// chatMaxTokens limita custo/latência do Chat genérico (hoje só expandQuery,
	// que devolve uma lista curta de termos — 500 é folgado para esse uso).
	chatMaxTokens = 500
	// chatSeed: OpenAI documenta seed em chat/completions para maior reprodutibilidade (não garantia absoluta).
	chatSeed = 42

	// classifyMaxTokens é o teto de ClassifyChat — maior que chatMaxTokens porque
	// o schema da classificação pode incluir evidence_highlights (até 8 blocos,
	// até 5 trechos de ~180 caracteres cada) e suggested_tags: um teto apertado
	// corta a resposta no meio do objeto JSON (finish_reason "length"), e um JSON
	// truncado é indistinguível de "resposta em formato inválido" no parse. Só
	// limita o teto — não força o modelo a gastar mais quando a resposta é curta.
	classifyMaxTokens = 900

	// Parâmetros do insight estratégico (2–4 frases em PT-BR; max_tokens alinhado ao teto em runes no Go).
	strategyInsightTemperature = 0.7
	strategyInsightMaxTokens   = 320

	maxOpenAIErrorBodyLog = 4096
)

// TokenUsage reflete o bloco "usage" da API OpenAI chat/completions.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResult é o conteúdo da primeira choice mais métricas de tokens.
type ChatResult struct {
	Content string
	Usage   TokenUsage
}

// LLMClient é um cliente HTTP puro para a API de Chat Completion da OpenAI.
// Sem SDK externo — mesma filosofia do embedder.go.
type LLMClient struct {
	apiKey  string
	model   string
	chatURL string
	client  *http.Client
}

func newLLMClient(apiKey string) *LLMClient {
	return &LLMClient{
		apiKey:  apiKey,
		model:   chatModel,
		chatURL: openAIChatURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Chat envia um system prompt e uma mensagem do usuário e retorna o conteúdo
// textual da primeira escolha gerada pelo modelo e o usage reportado pela API.
// temperature=0 garante saída determinística, essencial para o parse de JSON.
func (c *LLMClient) Chat(ctx context.Context, systemPrompt, userMsg string) (ChatResult, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type requestBody struct {
		Model            string    `json:"model"`
		Temperature      float64   `json:"temperature"`
		TopP             float64   `json:"top_p"`
		MaxTokens        int       `json:"max_tokens,omitempty"`
		PresencePenalty  float64   `json:"presence_penalty"`
		FrequencyPenalty float64   `json:"frequency_penalty"`
		Seed             int       `json:"seed"`
		Messages         []message `json:"messages"`
	}

	body := requestBody{
		Model:            c.model,
		Temperature:      chatTemperature,
		TopP:             chatTopP,
		MaxTokens:        chatMaxTokens,
		PresencePenalty:  0,
		FrequencyPenalty: 0,
		Seed:             chatSeed,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: criar request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: executar request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: ler resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyLen := len(rawBody)
		bodyForLog := string(rawBody)
		if len(bodyForLog) > maxOpenAIErrorBodyLog {
			bodyForLog = bodyForLog[:maxOpenAIErrorBodyLog] + "…(truncated)"
		}
		slog.Error("openai_chat_http_error",
			"status", resp.StatusCode,
			"model", c.model,
			"body_len", bodyLen,
			"body", bodyForLog,
		)
		return ChatResult{}, fmt.Errorf("llm: status %d: %s", resp.StatusCode, string(rawBody))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return ChatResult{}, fmt.Errorf("llm: parse response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("llm: resposta sem choices")
	}

	return ChatResult{
		Content: chatResp.Choices[0].Message.Content,
		Usage:   chatResp.Usage,
	}, nil
}

// ClassifyChat é o Chat da classificação de crédito (ClassifyExpense): mesma
// temperatura/seed determinística de Chat, mas com response_format
// json_object (a API OpenAI garante sintaticamente um objeto JSON puro,
// sem cerca de markdown nem prosa ao redor — reduz a causa mais comum de
// falha de parse) e classifyMaxTokens no lugar de chatMaxTokens. O prompt já
// instrui "responda apenas com JSON" (exigência da OpenAI para json_object
// funcionar). Não usar para expandQuery: aquele fluxo espera texto livre
// (lista de termos), não um objeto JSON.
func (c *LLMClient) ClassifyChat(ctx context.Context, systemPrompt, userMsg string) (ChatResult, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type responseFormat struct {
		Type string `json:"type"`
	}
	type requestBody struct {
		Model            string         `json:"model"`
		Temperature      float64        `json:"temperature"`
		TopP             float64        `json:"top_p"`
		MaxTokens        int            `json:"max_tokens,omitempty"`
		PresencePenalty  float64        `json:"presence_penalty"`
		FrequencyPenalty float64        `json:"frequency_penalty"`
		Seed             int            `json:"seed"`
		Messages         []message      `json:"messages"`
		ResponseFormat   responseFormat `json:"response_format"`
	}

	body := requestBody{
		Model:            c.model,
		Temperature:      chatTemperature,
		TopP:             chatTopP,
		MaxTokens:        classifyMaxTokens,
		PresencePenalty:  0,
		FrequencyPenalty: 0,
		Seed:             chatSeed,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
		ResponseFormat: responseFormat{Type: "json_object"},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: marshal classify request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: criar classify request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: executar classify request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: ler classify resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyLen := len(rawBody)
		bodyForLog := string(rawBody)
		if len(bodyForLog) > maxOpenAIErrorBodyLog {
			bodyForLog = bodyForLog[:maxOpenAIErrorBodyLog] + "…(truncated)"
		}
		slog.Error("openai_classify_chat_http_error",
			"status", resp.StatusCode,
			"model", c.model,
			"body_len", bodyLen,
			"body", bodyForLog,
		)
		return ChatResult{}, fmt.Errorf("llm: status %d: %s", resp.StatusCode, string(rawBody))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return ChatResult{}, fmt.Errorf("llm: parse classify response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("llm: resposta sem choices")
	}
	// finish_reason "length" = a API cortou a resposta no max_tokens; se isto
	// aparecer em produção com alguma frequência, classifyMaxTokens precisa
	// subir de novo — é o sinal mais direto de que o teto está apertado.
	if chatResp.Choices[0].FinishReason == "length" {
		slog.Warn("openai_classify_chat_truncated", "model", c.model, "max_tokens", classifyMaxTokens)
	}

	return ChatResult{
		Content: chatResp.Choices[0].Message.Content,
		Usage:   chatResp.Usage,
	}, nil
}

// StrategyInsightChat chama chat/completions com temperatura e max_tokens próprios ao insight
// (sem seed, para não forçar reprodutibilidade neste fluxo opcional).
func (c *LLMClient) StrategyInsightChat(ctx context.Context, systemPrompt, userMsg string) (ChatResult, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type requestBody struct {
		Model            string    `json:"model"`
		Temperature      float64   `json:"temperature"`
		TopP             float64   `json:"top_p"`
		MaxTokens        int       `json:"max_tokens,omitempty"`
		PresencePenalty  float64   `json:"presence_penalty"`
		FrequencyPenalty float64   `json:"frequency_penalty"`
		Messages         []message `json:"messages"`
	}

	body := requestBody{
		Model:            c.model,
		Temperature:      strategyInsightTemperature,
		TopP:             chatTopP,
		MaxTokens:        strategyInsightMaxTokens,
		PresencePenalty:  0,
		FrequencyPenalty: 0,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: marshal strategy request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: criar strategy request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: executar strategy request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: ler strategy resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyLen := len(rawBody)
		bodyForLog := string(rawBody)
		if len(bodyForLog) > maxOpenAIErrorBodyLog {
			bodyForLog = bodyForLog[:maxOpenAIErrorBodyLog] + "…(truncated)"
		}
		slog.Error("openai_strategy_chat_http_error",
			"status", resp.StatusCode,
			"model", c.model,
			"body_len", bodyLen,
			"body", bodyForLog,
		)
		return ChatResult{}, fmt.Errorf("llm: strategy status %d: %s", resp.StatusCode, string(rawBody))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return ChatResult{}, fmt.Errorf("llm: parse strategy response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("llm: strategy resposta sem choices")
	}

	if chatResp.Usage.TotalTokens > 0 {
		slog.Debug("openai_strategy_chat_usage",
			"prompt_tokens", chatResp.Usage.PromptTokens,
			"completion_tokens", chatResp.Usage.CompletionTokens,
			"total_tokens", chatResp.Usage.TotalTokens,
		)
	}

	return ChatResult{
		Content: chatResp.Choices[0].Message.Content,
		Usage:   chatResp.Usage,
	}, nil
}

const (
	leakEnrichTemperature = 0.2
	leakEnrichMaxTokens   = 1600
)

// LeakEnrichChat chama a API com response_format json_object para preencher reason/fix dos vazamentos.
func (c *LLMClient) LeakEnrichChat(ctx context.Context, systemPrompt, userMsg string) (ChatResult, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type responseFormat struct {
		Type string `json:"type"`
	}
	type requestBody struct {
		Model            string         `json:"model"`
		Temperature      float64        `json:"temperature"`
		TopP             float64        `json:"top_p"`
		MaxTokens        int            `json:"max_tokens,omitempty"`
		PresencePenalty  float64        `json:"presence_penalty"`
		FrequencyPenalty float64        `json:"frequency_penalty"`
		Messages         []message      `json:"messages"`
		ResponseFormat   responseFormat `json:"response_format"`
	}

	body := requestBody{
		Model:            c.model,
		Temperature:      leakEnrichTemperature,
		TopP:             chatTopP,
		MaxTokens:        leakEnrichMaxTokens,
		PresencePenalty:  0,
		FrequencyPenalty: 0,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
		ResponseFormat: responseFormat{Type: "json_object"},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: marshal leak request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: criar leak request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: executar leak request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResult{}, fmt.Errorf("llm: ler leak resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyForLog := string(rawBody)
		if len(bodyForLog) > maxOpenAIErrorBodyLog {
			bodyForLog = bodyForLog[:maxOpenAIErrorBodyLog] + "…"
		}
		slog.Error("openai_leak_chat_http_error",
			"status", resp.StatusCode,
			"model", c.model,
			"body", bodyForLog,
		)
		return ChatResult{}, fmt.Errorf("llm: leak status %d", resp.StatusCode)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return ChatResult{}, fmt.Errorf("llm: parse leak envelope: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("llm: leak resposta sem choices")
	}

	return ChatResult{
		Content: chatResp.Choices[0].Message.Content,
		Usage:   chatResp.Usage,
	}, nil
}
