# Legislação — PDFs oficiais e mapas artigo → página

Cada documento do corpus tem um par: o **PDF oficial** e o **mapa artigo → página**
(`*_article_page_map.json`), gerado por `scripts/legislacao/map_pdf_pages.py` e
consumido por `internal/ingestion/pdf_map.go` (`LoadLeiArticlePageMap`). O mapa
é o que sustenta a ancoragem "Ver lei" na UI e o QR do dossiê em PDF.

| Documento | PDF | Mapa | `lei_version` | Artigos |
|---|---|---|---|---|
| LC 214/2025 (compilado) | `leicomplementar-214-16-janeiro-2025-796905-normaatualizada-pl.pdf` | `lc214_article_page_map.json` | `2026-06-16-lc214-atualizada` | 580 |
| PLP 68/2024 (tramitação) | `DOC-PLP-682024-20240722.pdf` | `lc68_article_page_map.json` | `2024-07-22-doc-plp-68` | 700 no artefato, 494 reproduzíveis — ver `../../scripts/legislacao/KNOWN_DRIFT.md` |

`lei_version` precisa estar em `ExpectedLeiPDFMapVersions`
(`internal/ingestion/pdf_map.go`) — é uma **lista**, então acrescentar um
documento novo não invalida os anteriores.

## Por que o PDF da LC 214 é o "norma atualizada" da Câmara, e não o do DOU

O corpus limpo (`docs/lc214_2025_limpa.md`) vem do **texto compilado** do
Planalto, que já incorpora as alterações da LC 227/2026 — inclusive 36 artigos
inseridos ("Art. 323-A" e afins). O PDF do DOU de 16/01/2025 é a publicação
**original** e não contém esses artigos: a ancoragem "Ver lei" cairia na página
errada para eles, ou em página nenhuma.

O PDF do CEDI/Câmara (`...-normaatualizada-pl.pdf`, 298 páginas, gerado em
16/06/2026 segundo os metadados — o único carimbo de versão que ele traz)
corresponde ao mesmo texto: **580 artigos distintos nos dois artefatos, conjuntos
idênticos**, verificado por `internal/ingestion/pdf_map_lc214_test.go`.

Há ainda uma **republicação parcial em 23/01/2025** (correção no Anexo XXIII) —
já refletida tanto no compilado quanto neste PDF.

## Verificação ao trocar de PDF ou regenerar um mapa

1. Gerar sem `--check` (`--check` só faz sentido contra um artefato anterior do
   mesmo arquivo-fonte).
2. Rodar `go test ./internal/ingestion/ -run LC214` — trava que **todo**
   `article_id` do corpus tem entrada no mapa. Sem isso, uma divergência de
   normalização entre o parser Go e o script Python deixa artigos sem página
   **em silêncio** (`ApplyLeiArticlePageMap` só faz `continue`).
3. Conferir uma amostra abrindo o PDF na página que o mapa indica — não confiar
   no primeiro output. Na LC 214 foram 14 artigos conferidos (extremos, artigos
   com letra e os três que a regra de ruído "Vigência" apagava), 14/14 corretos.
