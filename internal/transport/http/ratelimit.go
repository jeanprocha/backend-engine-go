package http

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type clientLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// rateLimiter aplica token bucket por IP nas rotas que o envolvem.
// X-Forwarded-For só é considerado se TRUST_PROXY=true ou RATE_LIMIT_TRUST_X_FORWARDED_FOR=true
// (proxy à frente confiável); caso contrário o cliente pode forjar o IP.
type rateLimiter struct {
	cfg rateLimiterConfig
	mu  sync.Mutex
	ips map[string]*clientLimiterEntry
}

type rateLimiterConfig struct {
	enabled           bool
	rps               rate.Limit
	burst             int
	trustForwardedFor bool
	retryAfterSec     int
}

func loadRateLimiterConfig() rateLimiterConfig {
	cfg := rateLimiterConfig{
		enabled:           true,
		rps:               rate.Limit(0.5),
		burst:             5,
		trustForwardedFor: false,
		retryAfterSec:     2,
	}
	if v := strings.TrimSpace(os.Getenv("RATE_LIMIT_ENABLED")); v != "" {
		cfg.enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := strings.TrimSpace(os.Getenv("RATE_LIMIT_RPS")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cfg.rps = rate.Limit(f)
		}
	}
	if v := strings.TrimSpace(os.Getenv("RATE_LIMIT_BURST")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			cfg.burst = int(n)
		}
	}
	trust := strings.EqualFold(strings.TrimSpace(os.Getenv("RATE_LIMIT_TRUST_X_FORWARDED_FOR")), "true")
	trust = trust || strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_PROXY")), "true")
	cfg.trustForwardedFor = trust
	if v := strings.TrimSpace(os.Getenv("RATE_LIMIT_RETRY_AFTER_SEC")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			cfg.retryAfterSec = int(n)
		}
	}
	return cfg
}

func newRateLimiter() *rateLimiter {
	cfg := loadRateLimiterConfig()
	if !cfg.enabled {
		return &rateLimiter{cfg: cfg}
	}
	rl := &rateLimiter{cfg: cfg, ips: make(map[string]*clientLimiterEntry)}
	go rl.janitor()
	return rl
}

func (rl *rateLimiter) janitor() {
	for {
		time.Sleep(time.Minute)
		rl.mu.Lock()
		for ip, e := range rl.ips {
			if time.Since(e.lastSeen) > 3*time.Minute {
				delete(rl.ips, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) clientIP(r *http.Request) string {
	if rl.cfg.trustForwardedFor {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.ips[ip]
	if !ok {
		lim := rate.NewLimiter(rl.cfg.rps, rl.cfg.burst)
		rl.ips[ip] = &clientLimiterEntry{limiter: lim, lastSeen: time.Now()}
		return lim
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// Wrap aplica limite por IP; se RATE_LIMIT_ENABLED=false, devolve next sem alteração.
func (rl *rateLimiter) Wrap(next http.Handler) http.Handler {
	if !rl.cfg.enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.clientIP(r)
		lim := rl.getLimiter(ip)
		if !lim.Allow() {
			slog.Warn("rate_limit_exceeded",
				"ip", ip,
				"path", r.URL.Path,
				"method", r.Method,
			)
			w.Header().Set("Retry-After", strconv.Itoa(rl.cfg.retryAfterSec))
			writeError(w, http.StatusTooManyRequests, "Muitas requisicoes. Aguarde um momento.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// newTestRateLimiter cria limitador ativo para testes (alto RPS opcional, burst baixo).
func newTestRateLimiter(rps float64, burst int) *rateLimiter {
	cfg := rateLimiterConfig{
		enabled:       true,
		rps:           rate.Limit(rps),
		burst:         burst,
		retryAfterSec: 1,
	}
	rl := &rateLimiter{cfg: cfg, ips: make(map[string]*clientLimiterEntry)}
	go rl.janitor()
	return rl
}
