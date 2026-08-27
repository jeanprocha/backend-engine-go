# Scripts do mapa artigo → página do PDF oficial

Geram `docs/legislacao/lc68_article_page_map.json` (Opção C — ancoragem
"Ver lei" na UI), consumido por `internal/ingestion/pdf_map.go`.

```bash
pip install -r scripts/legislacao/requirements.txt

# Com o PDF oficial (posições reais, PyMuPDF):
python scripts/legislacao/map_pdf_pages.py \
  --pdf docs/legislacao/DOC-PLP-682024-20240722.pdf \
  --lei-version 2024-07-22-doc-plp-68 \
  --out docs/legislacao/lc68_article_page_map.json

# --check: só verifica se reproduz o artefato já versionado, não escreve.
python scripts/legislacao/map_pdf_pages.py \
  --pdf docs/legislacao/DOC-PLP-682024-20240722.pdf \
  --lei-version 2024-07-22-doc-plp-68 \
  --out docs/legislacao/lc68_article_page_map.json --check

# Sem o PDF (coordenadas ilustrativas — avisa em stderr, nunca usar em produção):
python scripts/legislacao/map_placeholder_from_md.py \
  --md docs/lc68_2024_limpa.md \
  --lei-version 2024-07-22-doc-plp-68 \
  --out docs/legislacao/lc68_article_page_map.json
```

`--lei-version` precisa coincidir com `ExpectedLeiPDFMapVersion` em
`internal/ingestion/pdf_map.go` — o `cmd/ingest` rejeita o mapa se não bater
(ver comentário na constante).

Ver `KNOWN_DRIFT.md` para o estado da reprodução contra o artefato atual
(`lc68_article_page_map.json`) — resumo: o algoritmo está correto onde
encontra o padrão; a divergência vem do PDF versionado ter mudado de
paginação desde que o mapa foi gerado, não do script.
