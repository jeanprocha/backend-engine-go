package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jeanprocha/backend-engine-go/internal/auth"
	"github.com/jeanprocha/backend-engine-go/internal/classifier"
	"github.com/jeanprocha/backend-engine-go/internal/company"
	"github.com/jeanprocha/backend-engine-go/internal/history"
	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
	"github.com/jeanprocha/backend-engine-go/internal/rag"
	"github.com/jeanprocha/backend-engine-go/internal/report"
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

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY nao definida")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL nao definida")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx := context.Background()

	store, err := ingestion.NewStore(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}
	defer store.Close()

	embedder := ingestion.NewEmbedder(apiKey)
	ragSvc := rag.NewService(store, embedder)
	taxEngine := tax.NewCalculator()
	classifierSvc := classifier.NewService(ragSvc, apiKey)
	histRepo := history.NewRepo(store.Pool())
	compRepo := company.NewRepo(store.Pool())

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

	srv := transporthttp.NewServer(":"+port, store, ragSvc, taxEngine, classifierSvc, histRepo, compRepo, transporthttp.AuthRouteConfig{
		DevSkip:  authSkip,
		Verifier: clerkVer,
	}, report.GenerateDiagnosticPDF)

	// Inicia o servidor em goroutine para não bloquear o handler de sinal.
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("servidor iniciado em :%s", port)
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
		log.Printf("sinal recebido: %s — encerrando...", sig)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	log.Println("servidor encerrado com sucesso")
	return nil
}
