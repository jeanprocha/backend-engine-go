package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
)

// SearchResult representa um chunk recuperado por similaridade semantica.
type SearchResult struct {
	ArticleID  string
	Content    string
	Metadata   map[string]string
	Similarity float64
}

// StorableChunk combina o chunk parseado com seu embedding gerado.
type StorableChunk struct {
	ArticleChunk
	Embedding []float32
}

// ErrArticleNotFound indica chunk inexistente ou sem metadata.article_id canónico.
var ErrArticleNotFound = errors.New("ingestion: law article not found")

// FullArticle concatena todos os chunks que partilham o mesmo título canónico em metadata.
type FullArticle struct {
	RequestedChunkID string
	Title            string
	Content          string
	Source           string
}

// Store gerencia a conexao com o banco e a persistencia dos chunks.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore cria um pool de conexoes e registra o tipo vector do pgvector.
// connStr deve ser a DATABASE_URL do Supabase no formato postgres://...
func NewStore(ctx context.Context, connStr string) (*Store, error) {
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("store: configuracao invalida: %w", err)
	}

	// Registra o tipo vector para cada nova conexao do pool.
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvector.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao criar pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: banco inacessivel: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close encerra o pool de conexoes.
func (s *Store) Close() {
	s.pool.Close()
}

// Ping verifica se o banco está acessível. Usado pelo health check da API.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Pool expõe o pool pgx para outros pacotes (ex.: histórico de simulações).
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Search executa busca semantica por similaridade de cosseno usando a funcao
// match_tax_law criada no Supabase.
//
// threshold: score minimo de similaridade (0.0 a 1.0). Recomendado: 0.5.
// limit: numero maximo de resultados.
func (s *Store) Search(ctx context.Context, embedding []float32, threshold float64, limit int) ([]SearchResult, error) {
	const query = `
		SELECT article_id, content, metadata, similarity
		FROM match_tax_law($1, $2, $3)
	`

	rows, err := s.pool.Query(ctx, query,
		pgvector.NewVector(embedding),
		threshold,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var (
			articleID  string
			content    string
			metaRaw    []byte
			similarity float64
		)

		if err := rows.Scan(&articleID, &content, &metaRaw, &similarity); err != nil {
			return nil, fmt.Errorf("store: scan result: %w", err)
		}

		var meta map[string]string
		if err := json.Unmarshal(metaRaw, &meta); err != nil {
			meta = map[string]string{}
		}

		results = append(results, SearchResult{
			ArticleID:  articleID,
			Content:    content,
			Metadata:   meta,
			Similarity: similarity,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterar resultados: %w", err)
	}

	return results, nil
}

// GetByIDs busca artigos pelo article_id exato, sem busca vetorial.
// Usado para injetar artigos âncora (regras gerais) no contexto da classificação.
// IDs não encontrados são silenciosamente omitidos — sem erro.
// Os resultados têm Similarity=1.0 para sinalizá-los como máxima relevância.
func (s *Store) GetByIDs(ctx context.Context, ids []string) ([]SearchResult, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	const query = `
		SELECT article_id, content, metadata
		FROM public.tax_law_chunks
		WHERE article_id = ANY($1)
	`

	rows, err := s.pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("store: getbyids query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var (
			articleID string
			content   string
			metaRaw   []byte
		)
		if err := rows.Scan(&articleID, &content, &metaRaw); err != nil {
			return nil, fmt.Errorf("store: getbyids scan: %w", err)
		}

		var meta map[string]string
		if err := json.Unmarshal(metaRaw, &meta); err != nil {
			meta = map[string]string{}
		}

		results = append(results, SearchResult{
			ArticleID:  articleID,
			Content:    content,
			Metadata:   meta,
			Similarity: 1.0,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: getbyids iterar: %w", err)
	}

	return results, nil
}

// GetFullArticleByChunkID resolve o metadata.article_id canónico a partir do ID da linha
// (ex.: lc68_0052_art_52_p2) e devolve o texto completo, ordenando partes por metadata.part.
func (s *Store) GetFullArticleByChunkID(ctx context.Context, chunkArticleID string) (*FullArticle, error) {
	if strings.TrimSpace(chunkArticleID) == "" {
		return nil, ErrArticleNotFound
	}

	const resolveCanon = `
		SELECT COALESCE(NULLIF(TRIM(metadata->>'article_id'), ''), '')
		FROM public.tax_law_chunks
		WHERE article_id = $1
		LIMIT 1
	`

	var canon string
	err := s.pool.QueryRow(ctx, resolveCanon, chunkArticleID).Scan(&canon)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrArticleNotFound
		}
		return nil, fmt.Errorf("store: resolver canon do chunk: %w", err)
	}
	if canon == "" {
		return nil, ErrArticleNotFound
	}

	const listChunks = `
		SELECT content, metadata
		FROM public.tax_law_chunks
		WHERE metadata->>'article_id' = $1
		ORDER BY
			CASE
				WHEN (metadata->>'part') ~ '^[0-9]+$' THEN (metadata->>'part')::int
				ELSE 0
			END,
			article_id ASC
	`

	rows, err := s.pool.Query(ctx, listChunks, canon)
	if err != nil {
		return nil, fmt.Errorf("store: listar chunks do artigo: %w", err)
	}
	defer rows.Close()

	var b strings.Builder
	source := DefaultDocumentProfile().SourceLabel
	first := true

	for rows.Next() {
		var content string
		var metaRaw []byte
		if err := rows.Scan(&content, &metaRaw); err != nil {
			return nil, fmt.Errorf("store: scan chunk artigo: %w", err)
		}
		var meta map[string]string
		if err := json.Unmarshal(metaRaw, &meta); err != nil {
			meta = map[string]string{}
		}
		if first {
			if v := strings.TrimSpace(meta["source"]); v != "" {
				source = v
			}
			first = false
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(content)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterar chunks artigo: %w", err)
	}

	if b.Len() == 0 {
		return nil, ErrArticleNotFound
	}

	return &FullArticle{
		RequestedChunkID: chunkArticleID,
		Title:            canon,
		Content:          b.String(),
		Source:           source,
	}, nil
}

// ErrPdfAnchorNotFound indica chunk sem pdf_page no metadata (mapa não aplicado ou artigo fora do mapa).
var ErrPdfAnchorNotFound = errors.New("ingestion: pdf anchor not available for chunk")

// GetPdfAnchorForChunk lê pdf_page e pdf_coord_y do metadata da linha do chunk.
func (s *Store) GetPdfAnchorForChunk(ctx context.Context, chunkArticleID string) (page int, coordY, convention, leiVersion string, err error) {
	chunkArticleID = strings.TrimSpace(chunkArticleID)
	if chunkArticleID == "" {
		return 0, "", "", "", ErrPdfAnchorNotFound
	}
	const q = `
		SELECT metadata->>'pdf_page', metadata->>'pdf_coord_y', metadata->>'pdf_coord_convention', metadata->>'lei_pdf_version'
		FROM public.tax_law_chunks
		WHERE article_id = $1
		LIMIT 1
	`
	var pp, cy, conv, lv *string
	err = s.pool.QueryRow(ctx, q, chunkArticleID).Scan(&pp, &cy, &conv, &lv)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", "", "", ErrPdfAnchorNotFound
		}
		return 0, "", "", "", fmt.Errorf("store: pdf anchor: %w", err)
	}
	if pp == nil || strings.TrimSpace(*pp) == "" {
		return 0, "", "", "", ErrPdfAnchorNotFound
	}
	n, err := strconv.Atoi(strings.TrimSpace(*pp))
	if err != nil || n < 1 {
		return 0, "", "", "", ErrPdfAnchorNotFound
	}
	coordY = ""
	if cy != nil {
		coordY = strings.TrimSpace(*cy)
	}
	convention = PdfCoordConventionYNormalized01
	if conv != nil && strings.TrimSpace(*conv) != "" {
		convention = strings.TrimSpace(*conv)
	}
	if lv != nil {
		leiVersion = strings.TrimSpace(*lv)
	}
	return n, coordY, convention, leiVersion, nil
}

// SaveChunks persiste uma lista de chunks dentro de uma transacao.
//
// ON CONFLICT (article_id) DO NOTHING garante idempotencia: rodar o ingest
// novamente nao duplica registros ja existentes.
// Requer a constraint: UNIQUE (article_id) na tabela.
func (s *Store) SaveChunks(ctx context.Context, chunks []StorableChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	const query = `
		INSERT INTO public.tax_law_chunks (article_id, content, embedding, metadata)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (article_id) DO NOTHING
	`

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: iniciar transacao: %w", err)
	}
	// Rollback automatico se Commit nao for chamado (panic ou retorno de erro).
	defer tx.Rollback(ctx) //nolint:errcheck

	batch := &pgx.Batch{}

	for _, c := range chunks {
		meta, err := json.Marshal(c.Metadata)
		if err != nil {
			return fmt.Errorf("store: marshal metadata do chunk %q: %w", c.ID, err)
		}

		var embedding any
		if len(c.Embedding) > 0 {
			embedding = pgvector.NewVector(c.Embedding)
		}

		batch.Queue(query, c.ID, c.Content, embedding, meta)
	}

	br := tx.SendBatch(ctx, batch)

	// Iterar sobre os resultados e fechar e obrigatorio no pgx.
	// Chamar apenas br.Close() sem iterar nao garante execucao de todos os statements.
	var firstErr error
	for i, c := range chunks {
		if _, err := br.Exec(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("store: insert chunk %d (%s): %w", i, c.ID, err)
		}
	}

	if err := br.Close(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("store: fechar batch: %w", err)
	}

	if firstErr != nil {
		return firstErr
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}

	return nil
}
