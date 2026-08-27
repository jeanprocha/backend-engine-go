package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jeanprocha/backend-engine-go/internal/auth"
	"github.com/jeanprocha/backend-engine-go/internal/classifier"
	"github.com/jeanprocha/backend-engine-go/internal/company"
	"github.com/jeanprocha/backend-engine-go/internal/config"
	"github.com/jeanprocha/backend-engine-go/internal/history"
	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
	"github.com/jeanprocha/backend-engine-go/internal/plg"
	"github.com/jeanprocha/backend-engine-go/internal/rag"
	"github.com/jeanprocha/backend-engine-go/internal/report"
	"github.com/jeanprocha/backend-engine-go/internal/strategytags"
	"github.com/jeanprocha/backend-engine-go/internal/tax"
	transporthttp "github.com/jeanprocha/backend-engine-go/internal/transport/http"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("erro fatal: %v", err)
	}
}

func run() error {
	_ = godotenv.Load()

	initSlogFromEnv()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY nao definida")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL nao definida")
	}

	addr := config.ListenAddr()

	ctx := context.Background()

	store, err := ingestion.NewStore(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}
	defer store.Close()

	embedder := ingestion.NewEmbedder(apiKey)
	// RAG_DOCUMENT_PREFIX delimita a busca a um documento do corpus (W1/Onda 2,
	// PR 1). Vazio = corpus inteiro. Ver config.RAGDocumentPrefix para o
	// acoplamento com o documento corrente e a ordem em relação à migration 009.
	ragSvc := rag.NewService(store, embedder, config.RAGDocumentPrefix())
	taxEngine := tax.NewCalculator()
	// Classificador + insight pós-simulação (StrategyInsightChat). Desligar só o insight:
	// STRATEGY_INSIGHT_ENABLED=false ou 0 — ver handler_simulation.go (baseline de latência / stress).
	classifierSvc := classifier.NewService(ragSvc, apiKey)
	histRepo := history.NewRepo(store.Pool())
	compRepo := company.NewRepo(store.Pool())
	strategyTagRepo := strategytags.NewRepo(store.Pool())
	strategyTagCache := strategytags.NewListCache(3 * time.Minute)

	authSkip := os.Getenv("AUTH_SKIP") == "true"
	var clerkVer *auth.ClerkVerifier
	if !authSkip {
		jwksURL := os.Getenv("CLERK_JWKS_URL")
		if jwksURL == "" {
			return fmt.Errorf("CLERK_JWKS_URL obrigatoria (ou defina AUTH_SKIP=true para dev com X-User-ID)")
		}
		var err error
		clerkVer, err = auth.NewClerkVerifier(jwksURL)
		if err != nil {
			return fmt.Errorf("clerk jwks: %w", err)
		}
	}

	plgLimiter := plg.NewLimiterFromEnv()

	srv := transporthttp.NewServer(addr, store, ragSvc, taxEngine, classifierSvc, histRepo, compRepo, strategyTagRepo, strategyTagCache, transporthttp.AuthRouteConfig{
		DevSkip:  authSkip,
		Verifier: clerkVer,
		Plg:      plgLimiter,
	}, report.GenerateDiagnosticPDF)

	// Inicia o servidor em goroutine para não bloquear o handler de sinal.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("servidor iniciado", "addr", addr)
		if err := srv.Start(); !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Aguarda sinal de interrupção (Ctrl+C, kill, etc.) ou erro do servidor.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("servidor encerrou com erro: %w", err)
	case sig := <-quit:
		slog.Info("sinal recebido, encerrando", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	slog.Info("servidor encerrado com sucesso")
	return nil
}

// initSlogFromEnv configura o logger padrão (texto ou JSON) para observabilidade em produção.
// LOG_FORMAT: "json" | "text" (default text; se ENV=production e LOG_FORMAT vazio, usa json).
// LOG_LEVEL: debug | info | warn | error (default info).
func initSlogFromEnv() {
	format := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
	if format == "" && strings.ToLower(strings.TrimSpace(os.Getenv("ENV"))) == "production" {
		format = "json"
	}
	if format == "" {
		format = "text"
	}

	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(h))
}
