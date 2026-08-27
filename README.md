# backend-engine-go

Motor de simulacao tributaria em Go.

## Papel

Este repositorio existe para justificar o uso de Go com um motivo real:

- calculo deterministico
- simulacao por cenarios
- abatimento de creditos tributarios
- processamento em lote de servicos e despesas
- evolucao futura para concorrencia em cargas maiores

## Tese tecnica

Se este projeto virar apenas um CRUD fino, o uso de Go perde sentido. O valor dele esta no motor de simulacao e no processamento de cenarios.

O backend deve ser vendavel assim:

"Usei Go para isolar o motor de calculo, garantir previsibilidade nos calculos financeiros com decimal e abrir caminho para simulacoes em lote com muitas linhas de faturamento e despesas."

## Politica de precisao monetaria

- **Proibicao de `float64` no dominio fiscal do motor:** montantes, aliquotas e factores de transicao (ex.: tabela 2026-2033 em `internal/tax/transition_table.go`) nao passam por tipo de ponto flutuante. Usa-se `github.com/shopspring/decimal`; constantes e rampas sao definidas como **literais decimais em string** e `decimal.RequireFromString` — nunca `decimal.NewFromFloat` para esse dominio.
- **JSON:** valores monetarios e aliquotas expostas na API como **strings** (ex.: `"90.00"`), para o cliente nao perder precisao na deserializacao.
- **Excecoes fora do nucleo fiscal:** `float64`/`float32` permanecem aceitaveis onde o dominio nao e dinheiro nem aliquota — embeddings vetoriais, similaridade RAG, `confidence` de classificacao por IA, limitador de taxa. Isso nao entra no calculo de carga tributaria.
- **Mantra:** *IA explica; Go calcula.* A IA classifica despesas, explica e recupera trechos normativos; o motor em Go executa a matematica deterministica da simulacao. A LLM nao substitui o calculo de imposto.

## Classificador e RAG (limiar de retrieval)

- **`CLASSIFIER_RAG_THRESHOLD`** (opcional): limiar minimo de similaridade (0–1) para incluir chunks na busca semantica antes de montar o contexto da LLM. **Predefinicao:** `0.35`. Valores mais altos reduzem ruido e podem diminuir recall (menos trechos); valores mais baixos aumentam cobertura com risco de texto menos relevante. A UI do TribIA interpreta a banda **0,35–0,55** como «nexo ténue» para mensagens de transparencia (nao altera o motor numerico).

## Premissas de simulacao (delta e MEI)

- **Delta e delta_pct:** `delta = liquido projetado - liquido atual`. Positivo = custo adicional no cenario projetado; negativo = economia.
- **MEI:** so com `company_regime: mei` (sem inferencia pelo texto de `company_context`). Carga fixa mensal ilustrativa de R$ 85 (`MEIMonthlyDAS` em `internal/tax/rules.go`) — congelada como constante desde o W7/B2.2, nao mais overridavel por env (era `MEI_MONTHLY_DAS_BRL`, nunca configurada em producao). Sem fonte legal — o DAS real do MEI e uma fracao do salario minimo, nao um valor fixo em R$; premissa TribIA. Nao modela anexo nem teto. Ver `docs/migrations/004_delta_convention.sql` para historico antigo.

## Transicao 2026-2033, serie temporal e ISS municipal (legado)

### Modelo dual comparativo («overlap» no TribIA)

O motor **nao** mistura os dois regimes num unico passo antes do resultado. Para cada ano, executa **duas simulacoes completas e independentes**: carga **legada** (PIS/COFINS + ISS com factor municipal) e carga **destino** (CBS/IBS com creditos), ambas com `RulesForYear(ano)`. A transicao temporal entra pelos **fatores por ano** (reducao de PIS/COFINS, rampa CBS/IBS, ISS municipal), nao por uma soma arbitraria dos dois liquidos.

A resposta inclui `overlap_model: dual_comparative_v1` para o cliente e para prompts de IA alinharem a narrativa a este contrato.

**Semantica de `total_tax_net` na serie:** `old_tax_net + new_tax_net` serve a **altura do grafico empilhado** (dois blocos lado a lado na leitura comparativa); nao representa uma unica obrigacao tributaria «hibrida» calculada como um unico imposto.

- `POST /simulations` devolve `transition_series` com, por ano, `old_tax_net` / `new_tax_net`, `current` e `projected` (breakdown), `delta`, `delta_pct`, e `factors` (`pis_cofins_factor`, `cbs_rate`, `ibs_rate`, `combined_projected_rate`, `iss_municipal_factor`, `iss_model`).
- **ISS no regime legado:** a aliquota de ISS informada por linha de servico e multiplicada por `ISSMunicipalTransitionFactor()` (tabela em `internal/tax/transition_table.go`): 100% em 2026-2028; 90%, 80%, 70%, 60% em 2029-2032 (rampa de 1/10 ao ano, mesma proporcao do ICMS); 0% em 2033. Corrigido no W7/B2.2 — a versao anterior usava rampa de 1/5 ao ano (80/60/40/20), divergente do calendario legal.
- **Tabela de transicao (`internal/tax/transition_table.go`):** cada ano carrega a proveniencia do numero (`RuleBasis.Kind`: `lei_calendario`, `estimativa_oficial` ou `premissa_tribia`) — nenhum valor entra sem isso declarado. 2027-2028: PIS/COFINS extintos, CBS ~8,7% (referencia menos reducao compensatoria de 0,1 p.p.), IBS nominal em 0,1%. 2033: CBS 8,8% + IBS 17,7% = 26,5% (projecao oficial MF/TCU, ainda nao fixada em lei — a LC 214/2025 delega a fixacao a Resolucao do Senado). Ver `TransitionYearBasis(ano)`.
- **Historico antigo sem `factors` no JSONB:** `GET /simulation-records/{id}` aplica `enrichTransitionSeriesLegacy`: insere `TransitionYearFactors` via `RulesForYear(ano)` e preenche `current`/`projected` minimos a partir de `old_tax_net`/`new_tax_net` quando necessario — sem reexecutar o motor.

**Picos na serie («ano critico») no frontend PRO:** o motor devolve os pontos; o cliente calcula de forma deterministica o ano de maximo `new_tax_net` e o de maximo `delta` (ver `frontend-next/src/lib/transition-series-peaks.ts`). Nao ha endpoint dedicado.

### Validacao cruzada contra a Calculadora oficial da RFB (W7/B2.1)

`internal/tax/rfb_cross_test.go` compara CBS/IBS do motor TribIA contra a
Calculadora de Tributos RFB/Serpro para o caso canonico "empresa_servicos_padrao"
(`internal/tax/testdata/casos_canonicos.json`), ano a ano. Atras de build tag
`rfb` — nao compila nem roda em `go test ./...` normal, sem segredo nem
servico no CI.

**Antes de rodar:** os blocos "PREENCHER" no topo do arquivo tem placeholders
(copiados do exemplo de MERCADORIA da documentacao da RFB), nao valores
verificados — a API classifica por NCM+CST/cClassTrib, e servico usa NBS, nao
NCM. Descobrir os codigos reais simulando um servico na calculadora WEB
(`http://localhost:80`) e inspecionando a requisicao no DevTools antes de
confiar em qualquer resultado.

```bash
# 1. Instalar e subir a Calculadora offline (Docker ou JAR — link no topo do arquivo).
# 2. Confirmar a URL/path exatos em http://localhost:8080/api (Swagger).
# 3. Preencher os placeholders do arquivo com valores verificados.
go test -tags=rfb ./internal/tax/... -run TestRFB -v
# Para gravar a evidência em internal/enginevalidation/evidencia/validacao_rfb.json (consumida por
# GET /engine/validation, B2.3) — exige a versão da calculadora (ver abaixo):
RFB_CALCULADORA_VERSAO=<versao> go test -tags=rfb ./internal/tax/... -run TestRFB -v -rfb-update
```

**A evidência carimba a versão da Calculadora.** A calculadora é beta e muda de
versão: um artefato que não diz contra QUAL versão rodou sustenta no máximo
"motor validado", enquanto o selo do dossiê afirma "validado contra a versão X".
Por isso `internal/enginevalidation.Build` exige `calculadora_versao` (além dos
casos > 0, zero divergências e hash da tabela de transição) para reportar
`validated: true`, e `-rfb-update` **falha** em vez de gravar um artefato sem
versão. A versão é resolvida em duas etapas: `RFB_CALCULADORA_VERSAO` (override
manual — ler a versão na área "Dados Abertos" da UI da calculadora) e, se
ausente, um GET no path de versão, cujo valor default é **palpite não
confirmado** contra uma instância real (ajustável por
`RFB_CALCULADORA_VERSAO_PATH`) — mesma situação do path de cálculo.

## Contrato HTTP (OpenAPI minimo)

- Especificacao estatica em [`docs/openapi.yaml`](docs/openapi.yaml): `GET /health`, `POST /simulations`, esquema `ErrorResponse` (inclui `request_id` opcional para correlacao com logs).
- **Politica de versao:** rotas actuais permanecem sem prefixo `/v1/` ate decisao de produto; mudancas incompatíveis futuras devem introduzir novo prefixo (ex.: `/v2`) ou estrategia documentada no mesmo ficheiro — integradores devem tratar o YAML como referencia de estabilidade esperada, nao como substituto de testes de integracao.

## Autenticacao (Clerk)

- Rotas protegidas: `POST`/`GET /simulation-records`, `GET /simulation-records/{id}`, CRUD `/companies`.
- Producao: defina `CLERK_JWKS_URL` com a URL JWKS da instancia Clerk (Frontend API). O Next.js envia `Authorization: Bearer <session_jwt>`; o claim `sub` identifica o utilizador.
- Dev local sem validar JWT no Go: `AUTH_SKIP=true` aceita header `X-User-ID` (nao usar em producao).
- Rotas publicas: `GET /health`, `POST /simulations`, `POST /credit-classifications`, `POST /credit-classifications/batch`, `POST /ai/explanations`.

## Escopo inicial

- receber servicos, despesas e premissas
- calcular comparativo entre cenario atual e futuro
- calcular creditos tributarios sobre despesas elegiveis
- retornar consolidado e detalhamento
- expor API HTTP simples com `net/http`
- incluir um comando de ingestao da legislacao para alimentar o RAG

## Estrutura recomendada

```text
backend-engine-go/
  cmd/
    api/
      main.go
    ingest/
      main.go
  internal/
    tax/
      entity.go
      calculator.go
      rules.go
      calculator_test.go
    ingestion/
      parser.go
      chunker.go
      embedder.go
      store.go
      pipeline.go
    transport/
      http/
        handler.go
        dto.go
        response.go
    platform/
      config/
      logger/
  pkg/
    money/
```

## Regras de implementacao

- usar `github.com/shopspring/decimal` para valores monetarios e aliquotas
- nao usar `float64` em calculo financeiro
- manter a logica tributaria no dominio, nao no handler
- manter `main.go` magro, apenas para bootstrap e wiring
- usar `ServeMux` nativo do Go
- escrever testes antes ou junto da logica de calculo
- usar o comando `cmd/ingest` para processar a legislacao em vez de empurrar isso para scripts improvisados
- quebrar o texto legal por artigo e preservar metadados

## MVP tecnico minimo

O backend so cumpre seu papel se entregar:

- calculo bruto no regime atual
- calculo bruto no regime projetado
- calculo de creditos elegiveis
- consolidacao da carga liquida
- resposta HTTP com detalhamento por servico e por despesa
- ingestao funcional de um recorte da lei com persistencia vetorial

## O que nao fazer

- nao esconder regra de negocio em helper generico
- nao criar interfaces vazias sem necessidade
- nao usar framework HTTP so por costume
- nao terceirizar a logica central para codigo gerado por IA
- nao fazer chunking cego por caracteres no texto da lei

## Legislação (cleaner + ingest + re-ingestão)

1. Coloque o texto bruto em `lei-em-texto.txt` na raiz do módulo e rode o cleaner:
   - `go run ./cmd/cleaner` (gera `docs/lc68_2024_limpa.md`, perfil `camara-plp` por defeito)
   - Outro documento/fonte: `-in`, `-out`, `-profile` (`camara-plp` | `planalto-dou` | `none` — ver `cmd/cleaner/clean.go`).
2. Ingestao: `go run ./cmd/ingest` (padrao: `docs/lc68_2024_limpa.md`, documento LC 68/2024) ou `go run ./cmd/ingest -file=caminho/outro.md -id-prefix=lc214_ -source="LC 214/2025"` para um documento diferente (o prefixo evita colisão de `article_id` entre documentos no mesmo corpus). Requer `.env` com `OPENAI_API_KEY` e `DATABASE_URL`.
   - **Mapa PDF (ancoragem PRO):** por defeito o ingest lê `docs/legislacao/lc68_article_page_map.json` e grava `pdf_page`, `pdf_coord_y`, `lei_pdf_version` nos metadados. Gere o mapa com `python scripts/legislacao/map_pdf_pages.py --pdf docs/legislacao/DOC-PLP-682024-20240722.pdf --lei-version <versao>` (PyMuPDF; ver `scripts/legislacao/README.md`) ou, sem o binário, `python scripts/legislacao/map_placeholder_from_md.py --md docs/lc68_2024_limpa.md --lei-version <versao>` (coordenadas ilustrativas). Para desactivar: `go run ./cmd/ingest -pdfmap=""`.
   - **API / viewer:** defina `LAW_OFFICIAL_PDF_URL` com a URL pública do PDF (CORS permitindo o domínio do frontend) e, se o nome do arquivo não for `DOC-PLP-682024-20240722.pdf`, `LAW_OFFICIAL_PDF_FILE` também. O mesmo ficheiro deve ser o usado para gerar o mapa. `LC68_OFFICIAL_PDF_URL` (nome antigo) ainda funciona como fallback — remover quando o Railway estiver com o nome novo configurado.
3. **Apos mudar o cleaner ou o parser de metadados** (`internal/ingestion/legal_structure.go`, chunking por artigo), a tabela vetorial nao atualiza sozinha: o ingest usa `ON CONFLICT (article_id) DO NOTHING`. Para repopular com metadados hierárquicos (`article_label`, `paragraph`, `inciso`, `alinea`, `span_note`, `structure_version`) e **ancoragem PDF**:
   - No SQL Editor do Supabase: `TRUNCATE TABLE public.tax_law_chunks;`
   - Rode o `cmd/ingest` de novo (re-gera embeddings; consome API OpenAI).

## Deploy (Railway + Vercel)

- **Porta:** `PORT` é lida automaticamente (`internal/config`). Fallback `:8080`.
- **Health:** `GET /health` — **sempre HTTP 200** (liveness); corpo JSON com `status` e `db` (se `db` ≠ `ok`, investigar Supabase). `GET /ready` — **503** se a base não responder (readiness).
- **CORS:** defina `CORS_ALLOWED_ORIGINS` com as origens exactas do frontend (ex. `https://app.seudominio.com,https://*.vercel.app` **não** funciona por wildcard — liste URLs de preview Vercel ou use um subdomínio estável). Com `ENV=production`, lista vazia = sem CORS.
- **Docker:** `docker build -t tribia-api -f Dockerfile .` na raiz deste módulo. Ver `.env.example`.
- **Dossiê público** (`GET /public/simulation-records/{id}`) deve estar acessível no URL público do Railway; no Vercel, configure o mesmo host em `NEXT_PUBLIC_API_URL` (browser) e, opcionalmente, `ENGINE_BASE_URL` no route handler de proxy.
