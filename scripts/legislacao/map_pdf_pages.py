#!/usr/bin/env python3
"""Gera o mapa artigo -> página/coordenada do PDF oficial (Opção C).

O shape de saída é consumido por internal/ingestion/pdf_map.go
(LoadLeiArticlePageMap) e mesclado nos chunks do RAG por
ApplyLeiArticlePageMap, casando pela chave = article_id do chunk (o
título canônico gerado por internal/ingestion/parse.go, ex. "Art. 52.").

A chave TEM que ser gerada com a mesma regra do parser Go
(articleTitleFromSubmatch em parse.go): "Art. " + número +
(º/° se houver) + ("." se houver) — qualquer divergência de normalização
quebra o join em silêncio (ApplyLeiArticlePageMap só faz `continue`
quando a chave não existe no mapa).

Uso:
    python scripts/legislacao/map_pdf_pages.py \
        --pdf docs/legislacao/DOC-PLP-682024-20240722.pdf \
        --lei-version 2024-07-22-doc-plp-68 \
        --out docs/legislacao/lc68_article_page_map.json

    # Só verificar se o script reproduz o artefato já versionado (não escreve):
    python scripts/legislacao/map_pdf_pages.py \
        --pdf docs/legislacao/DOC-PLP-682024-20240722.pdf \
        --lei-version 2024-07-22-doc-plp-68 \
        --out docs/legislacao/lc68_article_page_map.json --check

Requer PyMuPDF (ver requirements.txt): pip install -r scripts/legislacao/requirements.txt
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys

import pymupdf

# Mesma âncora do parser Go (parse.go): "Art." maiúsculo, só no início de
# LINHA — evita casar referências inline como "nos termos do art. 5º".
ARTICLE_LINE_RE = re.compile(r"^Art\.\s*(\d+)([º°]?)(\.)?")


def canonical_title(match: re.Match) -> str:
    """Reproduz articleTitleFromSubmatch (parse.go) — a chave de join com o RAG."""
    num, ordinal, dot = match.group(1), match.group(2) or "", match.group(3) or ""
    return f"Art. {num}{ordinal}{dot}"


def extract_articles(pdf_path: str, prf_file: str) -> dict[str, dict]:
    """Varre o PDF por linha, casando o início de cada linha contra ARTICLE_LINE_RE.

    Dedup: a PRIMEIRA ocorrência (em ordem de página) vence — um documento de
    tramitação como o PLP 68/2024 repete o texto da lei em anexos/quadros
    comparativos; sem essa regra, a última cópia sobrescreveria a primeira.
    """
    doc = pymupdf.open(pdf_path)
    articles: dict[str, dict] = {}
    try:
        for page_index in range(doc.page_count):
            page = doc[page_index]
            page_number = page_index + 1  # 1-based, como o resto do pipeline (metadata.pdf_page)
            page_height = page.rect.height
            text_dict = page.get_text("dict")
            for block in text_dict.get("blocks", []):
                for line in block.get("lines", []):
                    text = "".join(span.get("text", "") for span in line.get("spans", [])).strip()
                    match = ARTICLE_LINE_RE.match(text)
                    if not match:
                        continue
                    title = canonical_title(match)
                    if title in articles:
                        continue  # primeira ocorrência já venceu
                    y0 = line["bbox"][1]
                    articles[title] = {
                        "page": page_number,
                        "pdf_coord_y": f"{y0 / page_height:.4f}",
                        "prf_file": prf_file,
                    }
    finally:
        doc.close()
    return articles


def build_payload(pdf_path: str, lei_version: str, prf_file: str) -> dict:
    return {
        "lei_version": lei_version,
        "prf_file": prf_file,
        "convention": "y_normalized_0_1",
        "articles": extract_articles(pdf_path, prf_file),
    }


def dump(payload: dict) -> str:
    # sort_keys=True + indent=2 + ensure_ascii=False: mesmo formato do
    # artefato já versionado (confirmado por inspeção byte a byte).
    return json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--pdf", required=True, help="caminho do PDF oficial")
    parser.add_argument("--out", default="docs/legislacao/lc68_article_page_map.json", help="caminho de saída do JSON")
    parser.add_argument("--lei-version", required=True, help="identificador de versão (ex.: 2024-07-22-doc-plp-68)")
    parser.add_argument("--prf-file", default=None, help="nome do arquivo PDF de referência (default: basename de --pdf)")
    parser.add_argument("--check", action="store_true", help="regenera em memória e compara com --out; sai 1 em drift, sem escrever")
    args = parser.parse_args()

    prf_file = args.prf_file or os.path.basename(args.pdf)
    payload = build_payload(args.pdf, args.lei_version, prf_file)
    rendered = dump(payload)

    print(f"{len(payload['articles'])} artigos encontrados em {args.pdf}", file=sys.stderr)

    if args.check:
        try:
            with open(args.out, "r", encoding="utf-8") as f:
                existing = f.read()
        except FileNotFoundError:
            print(f"--check: {args.out} não existe", file=sys.stderr)
            return 1
        if rendered == existing:
            print(f"--check: {args.out} reproduzido sem diferenças", file=sys.stderr)
            return 0
        print(f"--check: {args.out} DIVERGE do que o script gera agora", file=sys.stderr)
        return 1

    with open(args.out, "w", encoding="utf-8", newline="\n") as f:
        f.write(rendered)
    print(f"escrito {args.out}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
