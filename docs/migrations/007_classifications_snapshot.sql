-- Snapshot rico (evidências RAG, ai_metadata) para reidratar o dashboard como na 1.ª execução.
ALTER TABLE public.simulations
  ADD COLUMN IF NOT EXISTS classifications_snapshot jsonb;

COMMENT ON COLUMN public.simulations.classifications_snapshot IS
  'JSON: expense_classifications, service_classifications, ai_metadata, discovered_tags (opcional).';
