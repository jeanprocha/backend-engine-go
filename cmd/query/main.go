package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
	"github.com/joho/godotenv"
)

// Variaveis (arquivo .env ou sistema):
//
//	OPENAI_API_KEY  — para gerar o embedding da pergunta
//	DATABASE_URL    — Supabase Postgres
//
// Uso:
//
//	query.exe "nao-cumulatividade IBS"
//	query.exe --threshold 0.3 "servico de streaming gera credito de CBS?"
//	query.exe --threshold 0.2 --limit 10 "bens imateriais"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	threshold := flag.Float64("threshold", 0.33, "score minimo de similaridade (0.0 a 1.0)")
	limit := flag.Int("limit", 5, "numero maximo de resultados")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Uso: %s [flags] \"sua pergunta\"\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExemplos:\n")
		fmt.Fprintf(os.Stderr, "  %s \"como funciona a nao-cumulatividade do IBS?\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --threshold 0.2 \"apropriacao de creditos\"\n", os.Args[0])
	}

	flag.Parse()

	question := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if question == "" {
		flag.Usage()
		return fmt.Errorf("informe a pergunta apos as flags")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY nao definida")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL nao definida")
	}

	ctx := context.Background()

	store, err := ingestion.NewStore(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}
	defer store.Close()

	embedder := ingestion.NewEmbedder(apiKey)
	chunks := []ingestion.ArticleChunk{
		{ID: "query", Title: "query", Content: question},
	}

	storables, err := embedder.EmbedChunks(ctx, chunks)
	if err != nil {
		return fmt.Errorf("gerar embedding da pergunta: %w", err)
	}
	if len(storables) == 0 || len(storables[0].Embedding) == 0 {
		return fmt.Errorf("embedding retornado esta vazio")
	}

	fmt.Printf("Embedding gerado: %d dimensoes (esperado 1536 com text-embedding-3-small)\n",
		len(storables[0].Embedding))

	results, err := store.Search(ctx, storables[0].Embedding, *threshold, *limit)
	if err != nil {
		return fmt.Errorf("busca semantica: %w", err)
	}

	fmt.Printf("\nPergunta: %q\n", question)
	fmt.Printf("Threshold: %.2f | Limite: %d\n\n", *threshold, *limit)

	if len(results) == 0 {
		fmt.Println("Nenhum artigo encontrado acima do threshold.")
		fmt.Println("Tente reduzir --threshold ou reformular a pergunta (ex.: termos da lei).")
		return nil
	}

	for i, r := range results {
		preview := r.Content
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}

		fmt.Printf("[%d] Score: %.4f | %s\n", i+1, r.Similarity, r.ArticleID)
		fmt.Printf("    Tipo: %s\n", r.Metadata["type"])
		fmt.Printf("    %s\n\n", preview)
	}

	return nil
}
