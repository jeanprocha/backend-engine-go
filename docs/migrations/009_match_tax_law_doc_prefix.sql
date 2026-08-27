-- 009 — match_tax_law ganha filtro opcional por documento (W1/Onda 2, PR 1)
--
-- POR QUÊ
-- A busca semântica não filtra por documento. Hoje isso é inofensivo: o corpus
-- tem 377 chunks, todos com prefixo `lc68_`. Quando a LC 214/2025 for ingerida
-- ao lado da LC 68 (Onda 2/PR 5 — coexistência por prefixo, sem TRUNCATE), os
-- chunks das duas leis passam a competir na mesma busca. Como a LC 214 é em
-- grande parte a consolidação do texto do PLP 68, são quase-duplicatas
-- semânticas: a busca devolveria escolhas arbitrárias entre duas versões do
-- mesmo artigo, e uma citação poderia apontar o rascunho revogado enquanto o
-- selo do dossiê afirma "LC 214/2025".
--
-- O QUE MUDA
-- Novo parâmetro `doc_prefix text DEFAULT NULL`. NULL ou vazio = sem filtro,
-- comportamento IDÊNTICO ao de hoje. Preenchido (ex.: 'lc214_') = só chunks
-- cujo article_id começa com esse prefixo — a mesma identidade que
-- ingestion.DocumentProfile.IDPrefix grava (internal/ingestion/parse.go).
--
-- COMPATIBILIDADE (importante para a ordem de deploy)
-- A função de 3 argumentos é REMOVIDA e recriada com o 4º parâmetro tendo
-- DEFAULT. Com isso, uma chamada `match_tax_law($1,$2,$3)` continua resolvendo
-- para esta função com doc_prefix = NULL. O código Go em produção hoje faz
-- exatamente essa chamada de 3 argumentos e NÃO quebra ao aplicar esta
-- migration. O Go só passa o 4º argumento quando RAG_DOCUMENT_PREFIX está
-- configurado — ou seja, esta migration pode ser aplicada antes ou depois do
-- deploy, em qualquer ordem, sem janela de indisponibilidade.
--
-- NÃO adicionar o 4º parâmetro sem remover a versão de 3: as duas assinaturas
-- coexistiriam e uma chamada de 3 argumentos ficaria ambígua
-- ("function match_tax_law(...) is not unique").
--
-- O corpo abaixo preserva literalmente a definição que estava em produção
-- (capturada via pg_get_functiondef em 27/08/2026) — só a cláusula de filtro
-- foi acrescentada. As migrations versionadas deste repo começam em 002; esta
-- função nunca esteve sob controle de versão até aqui.
--
-- APLICAR MANUALMENTE no SQL Editor do Supabase (não há runner — ver CLAUDE.md).

BEGIN;

DROP FUNCTION IF EXISTS public.match_tax_law(vector, double precision, integer);

CREATE OR REPLACE FUNCTION public.match_tax_law(
  query_embedding vector,
  match_threshold double precision,
  match_count integer,
  doc_prefix text DEFAULT NULL
)
RETURNS TABLE(
  id uuid,
  article_id text,
  content text,
  metadata jsonb,
  similarity double precision
)
LANGUAGE plpgsql
AS $function$
begin
  return query
  select
    tax_law_chunks.id,
    tax_law_chunks.article_id,
    tax_law_chunks.content,
    tax_law_chunks.metadata,
    1 - (tax_law_chunks.embedding <=> query_embedding) as similarity
  from tax_law_chunks
  where 1 - (tax_law_chunks.embedding <=> query_embedding) > match_threshold
    -- NULL/vazio = sem filtro (comportamento anterior, preservado).
    and (doc_prefix is null or doc_prefix = '' or tax_law_chunks.article_id like doc_prefix || '%')
  order by tax_law_chunks.embedding <=> query_embedding
  limit match_count;
end;
$function$;

COMMIT;

-- VERIFICAÇÃO (rodar depois; a primeira deve devolver o mesmo que antes)
--   select count(*) from match_tax_law(
--     (select embedding from tax_law_chunks limit 1), 0.0, 5);
--   select count(*) from match_tax_law(
--     (select embedding from tax_law_chunks limit 1), 0.0, 5, 'lc68_');
--   -- prefixo inexistente deve devolver 0 linhas:
--   select count(*) from match_tax_law(
--     (select embedding from tax_law_chunks limit 1), 0.0, 5, 'lc999_');
