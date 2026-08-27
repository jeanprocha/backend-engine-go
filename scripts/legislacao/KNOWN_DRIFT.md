# Drift conhecido — `map_pdf_pages.py` vs. `lc68_article_page_map.json`

`map_pdf_pages.py` foi recriado do zero (PR 3/W1 — os scripts originais
estavam documentados no README mas ausentes do repo e do histórico git).
Rodando `--check` contra o artefato já versionado (`docs/legislacao/
lc68_article_page_map.json`, 700 entradas):

```
python scripts/legislacao/map_pdf_pages.py \
  --pdf docs/legislacao/DOC-PLP-682024-20240722.pdf \
  --lei-version 2024-07-22-doc-plp-68 \
  --out docs/legislacao/lc68_article_page_map.json --check
```

Resultado: **494 de 700 chaves reproduzidas** (`exit 1`, drift real).

## O que está provado correto

Das 494 chaves que o script encontra, **nenhuma diverge em página ou
coordenada** do artefato versionado — zero discrepâncias no conjunto em
comum. O algoritmo (âncora `^Art\.\s*(\d+)([º°]?)(\.)?` no início de linha,
dedup "primeira ocorrência vence", `pdf_coord_y = bbox.y0 / altura_da_página`)
está correto onde encontra o padrão.

## O que diverge, e por quê

As 206 chaves ausentes (ex.: `Art. 1.`, `Art. 10`, `Art. 106`–`114`) não são
um problema de cobertura do algoritmo — são um problema do **arquivo fonte**.
Investigado o caso `Art. 10` → página 68 (segundo o mapa versionado): o texto
da página 68 do `DOC-PLP-682024-20240722.pdf` **atualmente no repo** não
contém "Art. 10" em lugar nenhum — mostra conteúdo de outro artigo
inteiramente (referências a "art. 69", parágrafos de um artigo anterior).

Ou seja: **o PDF hoje versionado no repo não é a mesma versão/paginação**
que gerou o artefato JSON original. Alguém re-exportou/otimizou/re-salvou o
PDF em algum momento depois que o mapa foi gerado, e a paginação mudou —
não há como o script (nem nenhum outro) reproduzir um mapa gerado contra um
arquivo que não é mais o que está no repo.

## Decisão: não investigar mais fundo

Este PDF é uma cópia de **tramitação** do PLP 68/2024 (499 páginas, formato
da Câmara dos Deputados) — vai ser **aposentado** assim que a Onda 2 trocar
o corpus pelo texto oficial da LC 214/2025 sancionada (Planalto/DOU, um
documento bem mais simples: sem "quadro comparativo", sem anexos
duplicando o texto). Perseguir bit a bit uma reprodução exata de um
artefato gerado contra um arquivo-fonte que já não existe mais teria
retorno zero — o algoritmo já está provado correto onde tem o quê comparar.

## Ação para a Onda 2

Ao gerar o mapa da LC 214/2025 pela primeira vez, rodar `--check` não faz
sentido (não existe artefato anterior para comparar) — só gerar de verdade
(`map_pdf_pages.py` sem `--check`) e revisar manualmente uma amostra contra
o PDF antes de aplicar. Não assumir que o mapa da LC 68/2024 é referência
de qualidade para além do que já está ingerido hoje.
