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

// Uso: go run ./cmd/ingest [-file=caminho.md] [caminho.md]
// Sem -file e sem argumento posicional, usa docs/lc68_2024_limpa.md (cwd = raiz do backend-engine-go).
// O parser grava metadados hierárquicos (article_label, paragraph, inciso, alinea, span_note) em cada chunk.
// Para atualizar chunks já inseridos, TRUNCATE public.tax_law_chunks e volte a executar (ver README).
//
// Variaveis (arquivo .env na raiz do backend-engine-go ou variaveis do sistema):
//
//	OPENAI_API_KEY  — chave da OpenAI (embeddings: text-embedding-3-small)
//	DATABASE_URL    — URI do Postgres do Supabase com senha real (sem placeholder)
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Go nao le .env sozinho; carrega se existir (cwd = pasta onde voce rodou o comando).
	_ = godotenv.Load()

	// --- 1. Argumentos e variaveis de ambiente ---

	defaultLaw := "docs/lc68_2024_limpa.md"
	defaultPDFMap := "docs/legislacao/lc68_article_page_map.json"
	lawPath := flag.String("file", "", "caminho do .md da lei (padrao: "+defaultLaw+")")
	pdfMapPath := flag.String("pdfmap", defaultPDFMap, "mapa artigo→página PDF (JSON); vazio para omitir")
	flag.Parse()

	lawFile := strings.TrimSpace(*lawPath)
	if lawFile == "" {
		if args := flag.Args(); len(args) >= 1 {
			lawFile = strings.TrimSpace(args[0])
		} else {
			lawFile = defaultLaw
		}
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY nao definida")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL nao definida")
	}

	// --- 2. Leitura do arquivo da lei ---

	fmt.Printf("lendo %s...\n", lawFile)
	raw, err := os.ReadFile(lawFile)
	if err != nil {
		return fmt.Errorf("abrir arquivo: %w", err)
	}

	// --- 3. Parse: fatia o texto por artigos ---

	parser := ingestion.NewParser(string(raw))
	chunks := parser.ParseArticles()
	fmt.Printf("encontrados %d artigos\n", len(chunks))

	pdfMapFile := strings.TrimSpace(*pdfMapPath)
	if pdfMapFile != "" {
		m, err := ingestion.LoadLeiArticlePageMap(pdfMapFile)
		if err != nil {
			return fmt.Errorf("mapa PDF: %w", err)
		}
		ingestion.ApplyLeiArticlePageMap(chunks, m)
		fmt.Printf("mapa PDF aplicado (%s): %d artigos no mapa\n", m.LeiVersion, len(m.Articles))
	}

	if len(chunks) == 0 {
		return fmt.Errorf("nenhum artigo encontrado; verifique se o arquivo esta no formato esperado")
	}

	// --- 4. Conecta ao banco antes de iniciar embeddings ---
	// Conectar cedo permite detectar falhas de credencial antes de gastar tokens da API.

	ctx := context.Background()
	store, err := ingestion.NewStore(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}
	defer store.Close()

	// --- 5. Embed + Save por batch ---
	// Salvar apos cada batch garante que um erro no meio nao descarta o progresso.
	// Na proxima execucao, ON CONFLICT (article_id) DO NOTHING ignora o que ja existe.

	embedder := ingestion.NewEmbedder(apiKey)
	total := 0

	if err := embedder.EmbedAndSave(ctx, chunks, func(batch []ingestion.StorableChunk) error {
		if err := store.SaveChunks(ctx, batch); err != nil {
			return err
		}
		total += len(batch)
		fmt.Printf("  salvo: %d/%d chunks\n", total, len(chunks))
		return nil
	}); err != nil {
		return fmt.Errorf("pipeline embed+save: %w", err)
	}

	fmt.Printf("concluido: %d chunks persistidos em tax_law_chunks\n", total)
	return nil
}
