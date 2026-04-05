package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiter_Wrap_429AfterBurst(t *testing.T) {
	// RPS muito baixo e burst 1: segunda requisicao imediata deve ser 429.
	rl := newTestRateLimiter(0.01, 1)
	h := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	addr := "192.0.2.1:1234"
	var got429 bool
	var retryAfter string
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/simulations", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			retryAfter = rec.Header().Get("Retry-After")
			break
		}
	}
	if !got429 {
		t.Fatal("expected at least one 429 after exhausting burst")
	}
	if retryAfter == "" {
		t.Fatal("expected Retry-After header on 429")
	}
}

func TestRateLimiter_DisabledPassesThrough(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "false")
	rl := newRateLimiter()
	if rl.cfg.enabled {
		t.Fatal("test expects RATE_LIMIT_ENABLED=false")
	}
	calls := 0
	h := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "127.0.0.1:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i, rec.Code)
		}
	}
	if calls != 50 {
		t.Fatalf("handler calls: %d", calls)
	}
}
