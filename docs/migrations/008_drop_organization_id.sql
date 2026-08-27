-- organization_id nunca foi enviado pelo frontend e o handler que o lia
-- foi removido na FE-4 PR 4a (company_id assume o papel de vínculo com
-- cliente/carteira). Coluna órfã desde então.
ALTER TABLE public.simulations
  DROP COLUMN IF EXISTS organization_id;
