-- Persiste regime CBS/IBS por linha de despesa (LC 214/2025) para reidratar o dashboard.
-- Execute no SQL Editor do Supabase se ainda não aplicou.

ALTER TABLE public.simulation_items
  ADD COLUMN IF NOT EXISTS regime_type TEXT NOT NULL DEFAULT 'padrao';

COMMENT ON COLUMN public.simulation_items.regime_type IS
  'Regime tributário da linha: padrao | diferenciado_60 | reduzido_zero (despesas).';
