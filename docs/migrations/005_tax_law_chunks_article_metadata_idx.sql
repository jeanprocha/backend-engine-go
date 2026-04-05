-- Índice para GET /law/articles/{id}: agregação por metadata.article_id (título canónico).
-- Opcional mas recomendado em produção com muitos chunks.
CREATE INDEX IF NOT EXISTS idx_tax_law_chunks_metadata_article_id
  ON public.tax_law_chunks ((metadata->>'article_id'));
