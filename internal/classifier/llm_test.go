package classifier

import (
	"context"
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
