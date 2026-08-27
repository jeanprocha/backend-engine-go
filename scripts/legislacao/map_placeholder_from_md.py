#!/usr/bin/env python3
"""Gera um mapa artigo -> página/coordenada ILUSTRATIVO, sem o PDF oficial.

Serve para não bloquear o pipeline (cmd/ingest aceita -pdfmap="" para omitir
de vez, mas às vezes é útil ter *algum* valor, ex. para exercitar a UI de
ancoragem em ambiente sem o binário do PDF). As coordenadas aqui NÃO
correspondem a uma posição real no documento — avisa nisso em stderr.

Diferente de map_pdf_pages.py: este script lê o .md JÁ LIMPO (saída de
cmd/cleaner) em vez do PDF, e extrai os títulos "#### Art. N" na ordem em
que aparecem — não há coordenada real para calcular, então distribui
página/posição sinteticamente (1 "página" a cada N artigos).

Uso:
    python scripts/legislacao/map_placeholder_from_md.py \
        --md docs/lc68_2024_limpa.md \
        --lei-version 2024-07-22-doc-plp-68 \
        --out docs/legislacao/lc68_article_page_map.json
"""
from __future__ import annotations

import argparse
import json
import re
import sys

# Mesma âncora que o parser Go usa para reconhecer o cabeçalho de artigo no .md.
HEADER_RE = re.compile(r"^#+\s*Art\.\s*(\d+)([º°]?)(\.)?")

ARTICLES_PER_PAGE = 4  # só para espalhar os artigos em "páginas" plausíveis


def canonical_title(match: re.Match) -> str:
    num, ordinal, dot = match.group(1), match.group(2) or "", match.group(3) or ""
    return f"Art. {num}{ordinal}{dot}"


def extract_titles_in_order(md_text: str) -> list[str]:
    titles: list[str] = []
    for line in md_text.splitlines():
        match = HEADER_RE.match(line.strip())
        if match:
            titles.append(canonical_title(match))
    return titles


def build_payload(md_text: str, lei_version: str, prf_file: str) -> dict:
    titles = extract_titles_in_order(md_text)
    articles: dict[str, dict] = {}
    for i, title in enumerate(titles):
        if title in articles:
            continue  # primeira ocorrência vence, mesma regra do script real
        articles[title] = {
            "page": (i // ARTICLES_PER_PAGE) + 1,
            "pdf_coord_y": "0.1000",
            "prf_file": prf_file,
        }
    return {
        "lei_version": lei_version,
        "prf_file": prf_file,
        "convention": "y_normalized_0_1",
        "articles": articles,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--md", required=True, help="caminho do .md limpo (saída de cmd/cleaner)")
    parser.add_argument("--out", default="docs/legislacao/lc68_article_page_map.json", help="caminho de saída do JSON")
    parser.add_argument("--lei-version", required=True, help="identificador de versão")
    parser.add_argument("--prf-file", default="placeholder.pdf", help="nome do PDF de referência (ilustrativo)")
    args = parser.parse_args()

    with open(args.md, "r", encoding="utf-8") as f:
        md_text = f.read()

    payload = build_payload(md_text, args.lei_version, args.prf_file)
    rendered = json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True) + "\n"

    with open(args.out, "w", encoding="utf-8", newline="\n") as f:
        f.write(rendered)

    print(
        f"AVISO: {len(payload['articles'])} artigos com coordenadas ILUSTRATIVAS "
        "(não correspondem a posição real no PDF) — o CTA 'Ver lei' levaria o "
        "usuário à página errada. Use map_pdf_pages.py com o PDF real assim que possível.",
        file=sys.stderr,
    )
    print(f"escrito {args.out}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
