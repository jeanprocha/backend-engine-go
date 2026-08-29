package classifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLLMClient_Chat_Usage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &LLMClient{
		apiKey:  "test-key",
		model:   "gpt-test",
		chatURL: srv.URL + "/v1/chat/completions",
		client:  srv.Client(),
	}

	cr, err := c.Chat(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if want := "{\"ok\":true}"; strings.TrimSpace(cr.Content) != want {
		t.Fatalf("content: got %q want %q", cr.Content, want)
	}
	if cr.Usage.PromptTokens != 10 || cr.Usage.CompletionTokens != 5 || cr.Usage.TotalTokens != 15 {
		t.Fatalf("usage: %+v", cr.Usage)
	}
}

func TestLLMClient_Chat_HTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limit"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &LLMClient{
		apiKey:  "test-key",
		model:   "gpt-test",
		chatURL: srv.URL + "/v1/chat/completions",
		client:  srv.Client(),
	}

	_, err := c.Chat(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error should mention status: %v", err)
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("error should include body: %v", err)
	}
}

// TestLLMClient_ClassifyChat_UsaResponseFormatJSONObject trava o que
// diferencia ClassifyChat de Chat: response_format json_object (a API OpenAI
// garante um objeto JSON puro, sem cerca de markdown nem prosa ao redor —
// a causa mais comum de "resposta em formato inválido") e classifyMaxTokens
// no lugar de chatMaxTokens (o schema de classificação é mais rico que o de
// expandQuery e cabia apertado no teto antigo).
func TestLLMClient_ClassifyChat_UsaResponseFormatJSONObject(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"is_eligible\":true}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &LLMClient{
		apiKey:  "test-key",
		model:   "gpt-test",
		chatURL: srv.URL + "/v1/chat/completions",
		client:  srv.Client(),
	}

	cr, err := c.ClassifyChat(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("ClassifyChat: %v", err)
	}
	if want := `{"is_eligible":true}`; cr.Content != want {
		t.Fatalf("content: got %q want %q", cr.Content, want)
	}

	rf, _ := gotBody["response_format"].(map[string]any)
	if rf == nil || rf["type"] != "json_object" {
		t.Fatalf("response_format: got %#v, want json_object", gotBody["response_format"])
	}
	if got, want := gotBody["max_tokens"], float64(classifyMaxTokens); got != want {
		t.Fatalf("max_tokens: got %v, want %v", got, want)
	}
	if got := gotBody["seed"]; got != float64(chatSeed) {
		t.Fatalf("seed: got %v, want %v (mesma reprodutibilidade de Chat)", got, chatSeed)
	}
}

// TestLLMClient_ClassifyChat_FinishReasonLength garante que uma resposta
// cortada pelo teto de tokens não vira erro (o Content parcial ainda é
// devolvido ao chamador — quem decide o que fazer é o parse em service.go),
// só loga o aviso openai_classify_chat_truncated.
func TestLLMClient_ClassifyChat_FinishReasonLength(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"is_eligible\":tr"},"finish_reason":"length"}],"usage":{"total_tokens":900}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &LLMClient{
		apiKey:  "test-key",
		model:   "gpt-test",
		chatURL: srv.URL + "/v1/chat/completions",
		client:  srv.Client(),
	}

	cr, err := c.ClassifyChat(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("ClassifyChat não deveria falhar por finish_reason=length: %v", err)
	}
	if cr.Content == "" {
		t.Fatal("Content vazio: o conteúdo parcial deveria continuar disponível para o chamador")
	}
}

func TestLLMClient_StrategyInsightChat(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Priorize créditos documentados."}}],"usage":{"prompt_tokens":20,"completion_tokens":8,"total_tokens":28}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &LLMClient{
		apiKey:  "test-key",
		model:   "gpt-test",
		chatURL: srv.URL + "/v1/chat/completions",
		client:  srv.Client(),
	}

	cr, err := c.StrategyInsightChat(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("StrategyInsightChat: %v", err)
	}
	if strings.TrimSpace(cr.Content) != "Priorize créditos documentados." {
		t.Fatalf("content: %q", cr.Content)
	}
}
