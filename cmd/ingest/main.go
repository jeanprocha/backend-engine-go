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

// Uso: go run ./cmd/ingest [-file=caminho.md] [-id-prefix=lc214_] [-source="LC 214/2025"] [caminho.md]
// Sem flags, usa docs/lc68_2024_limpa.md e o documento default (LC 68/2024) — o que está no banco hoje.
// O parser grava metadados hierárquicos (article_label, paragraph, inciso, alinea, span_note) em cada chunk.
//
// Múltiplos documentos no corpus: -id-prefix identifica o documento no article_id
// (chave única global da tabela), então dois documentos coexistem sem colisão.
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
	defaultDoc := ingestion.DefaultDocumentProfile()
	lawPath := flag.String("file", "", "caminho do .md da lei (padrao: "+defaultLaw+")")
	pdfMapPath := flag.String("pdfmap", defaultPDFMap, "mapa artigo→página PDF (JSON); vazio para omitir")
	idPrefix := flag.String("id-prefix", defaultDoc.IDPrefix, "prefixo do article_id — identifica o documento no corpus (ex.: lc214_)")
	sourceLabel := flag.String("source", defaultDoc.SourceLabel, "rótulo do documento em metadata.source (ex.: \"LC 214/2025\")")
	dryRun := flag.Bool("dry-run", false, "só mede: quantos chunks, quantos tokens e quanto custaria; não chama a OpenAI nem toca no banco")
	flag.Parse()

	lawFile := strings.TrimSpace(*lawPath)
	if lawFile == "" {
		if args := flag.Args(); len(args) >= 1 {
			lawFile = strings.TrimSpace(args[0])
		} else {
			lawFile = defaultLaw
		}
	}

	doc := ingestion.DocumentProfile{
		IDPrefix:    strings.TrimSpace(*idPrefix),
		SourceLabel: strings.TrimSpace(*sourceLabel),
	}
	// Valida antes de conectar ao banco e antes de gastar embeddings.
	if err := doc.Validate(); err != nil {
		return err
	}

	// As credenciais são exigidas só no caminho que realmente as usa: -dry-run
	// mede sem chave e sem banco, que é o ponto de poder medir antes de decidir.
	var apiKey, dbURL string
	if !*dryRun {
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("OPENAI_API_KEY nao definida")
		}

		dbURL = os.Getenv("DATABASE_URL")
		if dbURL == "" {
			return fmt.Errorf("DATABASE_URL nao definida")
		}
	}

	// --- 2. Leitura do arquivo da lei ---

	// Registro operacional da ingestão: prefixo e source determinam a
	// identidade dos chunks no corpus — precisam ficar no log da execução.
	fmt.Printf("lendo %s (documento: %s, id-prefix: %s)...\n", lawFile, doc.SourceLabel, doc.IDPrefix)
	raw, err := os.ReadFile(lawFile)
	if err != nil {
		return fmt.Errorf("abrir arquivo: %w", err)
	}

	// --- 3. Parse: fatia o texto por artigos ---

	parser := ingestion.NewParserForDocument(string(raw), doc)
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

	if *dryRun {
		reportDryRun(chunks)
		return nil
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

// Preço de text-embedding-3-small (o modelo em internal/ingestion/embedder.go),
// em dólares por 1 milhão de tokens. Se a OpenAI mudar a tabela, muda aqui — o
// número é declarado, não adivinhado em tempo de execução.
const (
	precoUSDPorMilhaoTokens  = 0.02
	tabelaDePrecoConferidaEm = "2026-08-27"

	// Faixa de caracteres por token para estimar sem tokenizador. Português
	// jurídico, com acentuação e palavras longas, tokeniza pior que inglês:
	// 3,0 é o cenário pessimista e 4,5 o otimista. Não usamos tiktoken porque
	// a diferença entre os extremos aqui é de centavos — o que a estimativa
	// precisa responder é "posso rodar sem pensar?", não o valor exato.
	charsPorTokenPessimista = 3.0
	charsPorTokenOtimista   = 4.5
)

// reportDryRun mede o que a ingestão custaria, sem chamar nada.
func reportDryRun(chunks []ingestion.ArticleChunk) {
	var totalChars int
	var maiorChunk int
	comPagina := 0
	porTipo := map[string]int{}

	for _, c := range chunks {
		n := len([]rune(c.Content))
		totalChars += n
		if n > maiorChunk {
			maiorChunk = n
		}
		if c.Metadata[ingestion.MetaPdfPage] != "" {
			comPagina++
		}
		porTipo[c.Metadata["type"]]++
	}

	tokensPess := float64(totalChars) / charsPorTokenPessimista
	tokensOtim := float64(totalChars) / charsPorTokenOtimista
	custoPess := tokensPess / 1_000_000 * precoUSDPorMilhaoTokens
	custoOtim := tokensOtim / 1_000_000 * precoUSDPorMilhaoTokens

	batches := (len(chunks) + 19) / 20 // defaultBatchSize = 20 no embedder

	fmt.Println()
	fmt.Println("─── DRY RUN — nada foi enviado à OpenAI nem gravado no banco ───")
	fmt.Printf("  chunks a inserir......: %d\n", len(chunks))
	for tipo, n := range porTipo {
		fmt.Printf("    tipo %-14s: %d\n", tipo, n)
	}
	fmt.Printf("  com pdf_page..........: %d de %d (%.1f%%)\n",
		comPagina, len(chunks), 100*float64(comPagina)/float64(len(chunks)))
	fmt.Printf("  caracteres totais.....: %d\n", totalChars)
	fmt.Printf("  maior chunk...........: %d caracteres\n", maiorChunk)
	fmt.Printf("  requisições à OpenAI..: %d (batches de 20)\n", batches)
	fmt.Println()
	fmt.Printf("  tokens estimados......: %.0f a %.0f\n", tokensOtim, tokensPess)
	fmt.Printf("  CUSTO ESTIMADO........: US$ %.4f a US$ %.4f\n", custoOtim, custoPess)
	fmt.Printf("    (%s a US$ %.2f/1M tokens, tabela conferida em %s)\n",
		"text-embedding-3-small", precoUSDPorMilhaoTokens, tabelaDePrecoConferidaEm)
	fmt.Println()
	fmt.Println("  Para executar de verdade, rode o mesmo comando sem -dry-run.")
}
