package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

// Service orquestra embed → search para responder perguntas sobre a legislação.
// É a extração da lógica do cmd/query para uso pela camada HTTP.
type Service struct {
	store    *ingestion.Store
	embedder *ingestion.Embedder
	// docPrefix delimita a busca a um documento legal (prefixo de article_id,
	// ex.: "lc214_"). Vazio = corpus inteiro, comportamento de sempre.
	// Injetado por quem monta o Service — este pacote não lê env, mesma regra
	// de internal/lawcorpus.
	docPrefix string
}

// NewService cria um Service com as dependências injetadas.
//
// docPrefix (W1/Onda 2, PR 1): delimita a busca semântica a um documento do
// corpus. Vazio = sem filtro. Enquanto só a LC 68 está ingerida isto é inócuo;
// passa a importar quando a LC 214/2025 coexistir no mesmo `tax_law_chunks`,
// porque as duas leis são quase-duplicatas semânticas e a busca sem filtro
// devolveria escolhas arbitrárias entre elas. Ver docs/migrations/009.
func NewService(store *ingestion.Store, embedder *ingestion.Embedder, docPrefix string) *Service {
	return &Service{store: store, embedder: embedder, docPrefix: strings.TrimSpace(docPrefix)}
}

// DocumentPrefix devolve o prefixo a que este Service delimita a busca ("" =
// corpus inteiro). Serve à observabilidade — o handler loga o escopo efetivo.
func (s *Service) DocumentPrefix() string { return s.docPrefix }

// Query gera o embedding da pergunta e executa a busca semântica no banco.
// threshold: score mínimo de similaridade (0.0 a 1.0), recomendado 0.33.
// limit: número máximo de resultados.
func (s *Service) Query(ctx context.Context, question string, threshold float64, limit int) ([]ingestion.SearchResult, error) {
	chunks := []ingestion.ArticleChunk{
		{ID: "query", Title: "query", Content: question},
	}

	storables, err := s.embedder.EmbedChunks(ctx, chunks)
	if err != nil {
		return nil, fmt.Errorf("rag: gerar embedding: %w", err)
	}
	if len(storables) == 0 || len(storables[0].Embedding) == 0 {
		return nil, fmt.Errorf("rag: embedding retornado vazio")
	}

	results, err := s.store.Search(ctx, storables[0].Embedding, threshold, limit, s.docPrefix)
	if err != nil {
		return nil, fmt.Errorf("rag: busca semantica: %w", err)
	}

	return results, nil
}

// GetByIDs é um passthrough para store.GetByIDs que permite ao pacote classifier
// buscar artigos âncora por ID sem acesso direto ao store.
func (s *Service) GetByIDs(ctx context.Context, ids []string) ([]ingestion.SearchResult, error) {
	results, err := s.store.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("rag: getbyids: %w", err)
	}
	return results, nil
}
