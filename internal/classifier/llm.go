package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	openAIChatURL = "https://api.openai.com/v1/chat/completions"
	chatModel     = "gpt-4o-mini"
)

// LLMClient é um cliente HTTP puro para a API de Chat Completion da OpenAI.
// Sem SDK externo — mesma filosofia do embedder.go.
type LLMClient struct {
	apiKey string
	model  string
	client *http.Client
}

func newLLMClient(apiKey string) *LLMClient {
	return &LLMClient{
		apiKey: apiKey,
		model:  chatModel,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Chat envia um system prompt e uma mensagem do usuário e retorna o conteúdo
// textual da primeira escolha gerada pelo modelo.
// temperature=0 garante saída determinística, essencial para o parse de JSON.
func (c *LLMClient) Chat(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type requestBody struct {
		Model       string    `json:"model"`
		Temperature float64   `json:"temperature"`
		Messages    []message `json:"messages"`
	}

	body := requestBody{
		Model:       c.model,
		Temperature: 0,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIChatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("llm: criar request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: executar request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm: ler resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: status %d: %s", resp.StatusCode, string(rawBody))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return "", fmt.Errorf("llm: parse response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm: resposta sem choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
