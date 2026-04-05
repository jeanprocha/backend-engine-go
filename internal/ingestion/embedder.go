package ingestion

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
	openAIEmbeddingsURL = "https://api.openai.com/v1/embeddings"

	// text-embedding-3-small gera vetores de 1536 dimensoes.
	// GPT-4o-mini e um modelo de chat, nao gera embeddings.
	embeddingModel = "text-embedding-3-small"

	// Limite seguro por request para nao estourar a janela de tokens da API.
	defaultBatchSize = 20

	maxRetries    = 3
	retryWaitBase = 2 * time.Second
)

// Embedder converte chunks de texto em vetores via API da OpenAI.
type Embedder struct {
	apiKey    string
	batchSize int
	client    *http.Client
}

// NewEmbedder cria um Embedder pronto para uso.
// apiKey e a OPENAI_API_KEY da sua conta.
func NewEmbedder(apiKey string) *Embedder {
	return &Embedder{
		apiKey:    apiKey,
		batchSize: defaultBatchSize,
		client:    &http.Client{Timeout: 60 * time.Second},
	}
}

// EmbedChunks recebe os chunks parseados e retorna StorableChunks com o embedding preenchido.
// Processa em batches para respeitar os limites da API.
// Se um batch falhar com 429 ou 5xx, faz retry com backoff exponencial (ate 3 tentativas).
func (e *Embedder) EmbedChunks(ctx context.Context, chunks []ArticleChunk) ([]StorableChunk, error) {
	result := make([]StorableChunk, 0, len(chunks))
	err := e.EmbedAndSave(ctx, chunks, func(batch []StorableChunk) error {
		result = append(result, batch...)
		return nil
	})
	return result, err
}

// EmbedAndSave processa os chunks em batches e chama onBatch apos cada um.
// Usar este metodo em vez de EmbedChunks garante que um erro nao descarta
// o progresso dos batches anteriores — o caller salva a cada batch concluido.
func (e *Embedder) EmbedAndSave(ctx context.Context, chunks []ArticleChunk, onBatch func([]StorableChunk) error) error {
	for i := 0; i < len(chunks); i += e.batchSize {
		end := i + e.batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		fmt.Printf("  embedding batch %d-%d de %d...\n", i+1, end, len(chunks))

		embeddings, err := e.requestWithRetry(ctx, batch)
		if err != nil {
			return fmt.Errorf("embedder: batch %d-%d: %w", i+1, end, err)
		}

		storables := make([]StorableChunk, len(batch))
		for j, chunk := range batch {
			storables[j] = StorableChunk{
				ArticleChunk: chunk,
				Embedding:    embeddings[j],
			}
		}

		if err := onBatch(storables); err != nil {
			return fmt.Errorf("embedder: callback batch %d-%d: %w", i+1, end, err)
		}
	}

	return nil
}

// requestWithRetry envolve requestEmbeddings com retry para erros transientes.
// Retenta em: 429 (rate limit) e qualquer 5xx (erro no servidor da OpenAI).
// Usa backoff exponencial: 2s, 4s, 8s.
func (e *Embedder) requestWithRetry(ctx context.Context, chunks []ArticleChunk) ([][]float32, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		embeddings, err := e.requestEmbeddings(ctx, chunks)
		if err == nil {
			return embeddings, nil
		}

		lastErr = err

		// Só faz retry em erros transientes; erros de autenticacao (401, 403)
		// e payload invalido (400) nao se resolvem esperando.
		if !isRetryable(err) {
			return nil, err
		}

		wait := retryWaitBase * time.Duration(attempt)
		fmt.Printf("    tentativa %d/%d falhou (%v); aguardando %s...\n",
			attempt, maxRetries, err, wait)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	return nil, fmt.Errorf("todas as %d tentativas falharam: %w", maxRetries, lastErr)
}

// isRetryable retorna true para erros de rate limit (429) e erros de servidor (5xx).
type httpStatusError struct {
	status int
	body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("openai status %d: %s", e.status, e.body)
}

func isRetryable(err error) bool {
	var se *httpStatusError
	if ok := asHTTPStatusError(err, &se); ok {
		return se.status == http.StatusTooManyRequests || se.status >= 500
	}
	return false
}

// asHTTPStatusError faz unwrap manual sem depender de errors.As para manter
// compatibilidade com o wrapping feito em requestEmbeddings.
func asHTTPStatusError(err error, target **httpStatusError) bool {
	if err == nil {
		return false
	}
	if se, ok := err.(*httpStatusError); ok {
		*target = se
		return true
	}
	return false
}

// --- tipos internos para comunicacao com a API ---

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// requestEmbeddings envia um batch de chunks para a API e retorna os vetores
// na mesma ordem dos chunks recebidos.
func (e *Embedder) requestEmbeddings(ctx context.Context, chunks []ArticleChunk) ([][]float32, error) {
	inputs := make([]string, len(chunks))
	for i, c := range chunks {
		inputs[i] = c.Content
	}

	body, err := json.Marshal(embeddingRequest{
		Model: embeddingModel,
		Input: inputs,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEmbeddingsURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("criar request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ler response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{status: resp.StatusCode, body: string(raw)}
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if parsed.Error != nil {
		return nil, fmt.Errorf("openai error [%s]: %s", parsed.Error.Type, parsed.Error.Message)
	}

	// A API garante retorno na mesma ordem do input,
	// mas usamos o Index para ser seguros.
	embeddings := make([][]float32, len(chunks))
	for _, d := range parsed.Data {
		embeddings[d.Index] = d.Embedding
	}

	for i, emb := range embeddings {
		if emb == nil {
			return nil, fmt.Errorf("embedding ausente para o chunk %d (%s)", i, chunks[i].ID)
		}
	}

	return embeddings, nil
}
