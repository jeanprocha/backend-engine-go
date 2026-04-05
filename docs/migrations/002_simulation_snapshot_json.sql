-- Execute no SQL Editor do Supabase após criar simulations/simulation_items.
-- Guarda o JSON completo do resultado (gross, credits, net, delta_pct) para reidratar o dashboard.

ALTER TABLE public.simulations
  ADD COLUMN IF NOT EXISTS simulation_snapshot JSONB;

COMMENT ON COLUMN public.simulations.simulation_snapshot IS
  'Resposta completa do motor de simulação (JSON) para histórico fiel no frontend.';
