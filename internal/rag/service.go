package rag

import (
	"context"
	"fmt"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

// Service orquestra embed → search para responder perguntas sobre a legislação.
// É a extração da lógica do cmd/query para uso pela camada HTTP.
type Service struct {
	store    *ingestion.Store
	embedder *ingestion.Embedder
}

// NewService cria um Service com as dependências injetadas.
func NewService(store *ingestion.Store, embedder *ingestion.Embedder) *Service {
	return &Service{store: store, embedder: embedder}
}

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

	results, err := s.store.Search(ctx, storables[0].Embedding, threshold, limit)
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
