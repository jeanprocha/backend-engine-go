package ingestion

import (
	"context"
	"fmt"
)

// CorpusDocumentRow agrega os chunks de tax_law_chunks por documento
// (metadata.source) — é a base de GET /law/corpus: reporta o que
// REALMENTE está ingerido, nunca o que se deseja ter.
type CorpusDocumentRow struct {
	Source           string
	LeiPDFVersion    string
	StructureVersion string
	// ChunkPrefix (ex.: "lc68_") — mesmo prefixo que DocumentProfile.IDPrefix
	// gravou no article_id de cada chunk deste documento.
	ChunkPrefix string
	Chunks      int
}

// ListCorpusDocuments agrupa os chunks ingeridos por documento (metadata.source).
func (s *Store) ListCorpusDocuments(ctx context.Context) ([]CorpusDocumentRow, error) {
	const q = `
		SELECT
			COALESCE(NULLIF(TRIM(metadata->>'source'), ''), '')                AS source,
			COALESCE(MIN(NULLIF(TRIM(metadata->>'lei_pdf_version'), '')), '')   AS lei_pdf_version,
			COALESCE(MIN(NULLIF(TRIM(metadata->>'structure_version'), '')), '') AS structure_version,
			COALESCE(MIN(split_part(article_id, '_', 1) || '_'), '')           AS chunk_prefix,
			COUNT(*)                                                           AS chunks
		FROM public.tax_law_chunks
		GROUP BY 1
		ORDER BY 5 DESC, 1 ASC
	`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store: list corpus documents: %w", err)
	}
	defer rows.Close()

	var out []CorpusDocumentRow
	for rows.Next() {
		var r CorpusDocumentRow
		if err := rows.Scan(&r.Source, &r.LeiPDFVersion, &r.StructureVersion, &r.ChunkPrefix, &r.Chunks); err != nil {
			return nil, fmt.Errorf("store: scan corpus document: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate corpus documents: %w", err)
	}
	return out, nil
}
