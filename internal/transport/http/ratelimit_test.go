package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestRateLimiter_PublicSimulationRecord_BurstThenLegitimateAccess cobre o
// critério de pronto da Etapa M/PR 11: burst num IP devolve 429 com a
// mensagem padrão, mas não afeta outro visitante legítimo (IP diferente)
// abrindo o mesmo dossiê — o link /public/simulation-records/{id} passou a
// ser divulgado publicamente pela landing (rota /exemplo).
func TestRateLimiter_PublicSimulationRecord_BurstThenLegitimateAccess(t *testing.T) {
	rl := newTestRateLimiter(0.01, 1)
	h := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	burstIP := "203.0.113.9:5555"
	path := "/public/simulation-records/11111111-1111-4111-8111-111111111111"

	req1 := httptest.NewRequest(http.MethodGet, path, nil)
	req1.RemoteAddr = burstIP
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("primeira requisicao do burst: status %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, path, nil)
	req2.RemoteAddr = burstIP
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("segunda requisicao do mesmo IP: esperado 429, obtido %d", rec2.Code)
	}
	if body := rec2.Body.String(); !strings.Contains(body, "Muitas requisicoes. Aguarde um momento.") {
		t.Fatalf("mensagem padrão de 429 ausente no corpo: %s", body)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Fatal("esperado header Retry-After no 429")
	}

	otherReq := httptest.NewRequest(http.MethodGet, path, nil)
	otherReq.RemoteAddr = "198.51.100.4:6666"
	otherRec := httptest.NewRecorder()
	h.ServeHTTP(otherRec, otherReq)
	if otherRec.Code != http.StatusOK {
		t.Fatalf("dossiê deve abrir normalmente para outro visitante (IP diferente): status %d", otherRec.Code)
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
