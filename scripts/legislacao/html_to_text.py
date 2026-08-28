#!/usr/bin/env python3
"""Extrai texto corrido do HTML oficial do Planalto para alimentar cmd/cleaner.

W1/Onda 2, PR 3. O `cmd/cleaner` espera texto bruto com UMA linha por bloco
(ele remonta parágrafos quebrados de PDF e ancora artigos com `#### Art. N`).
Este script produz exatamente isso a partir do HTML do Planalto, que é
FrontPage antigo: sem `<meta charset>`, codificado em ISO-8859-1, com o texto
em `<p class="MsoNormal">` e cada artigo precedido de `<a name="artN">`.

Decisões deliberadas:

- **Só stdlib.** `scripts/legislacao/requirements.txt` tem apenas PyMuPDF (para
  o mapa de páginas do PDF); não vale acrescentar lxml/BeautifulSoup por uma
  extração que `html.parser` resolve.
- **Uma linha por bloco.** Colapsar o espaço em branco DENTRO de cada bloco
  evita que o `Art. Nº` fique separado do resto por uma quebra interna (o HTML
  tem `Art. 1º<b>\\n\\t\\t</b>Ficam instituídos:`). É o que garante que a linha
  COMECE com "Art. N" e o cleaner consiga ancorar — a falha exata que deixou o
  "Art. 1º" da LC 68 sem âncora e `GET /law/articles/lc68_0001_art_1` em 404.
- **Não interpreta conteúdo jurídico.** Riscado (`<strike>`/`<del>`, texto
  revogado no compilado) é sinalizado, não decidido aqui: quem decide o que
  entra no corpus é a revisão humana da PR 3, não um heurístico.

Uso:
    python html_to_text.py -i lcp214.html -o lc214-em-texto.txt
    python html_to_text.py -i lcp214.html -o out.txt --encoding latin-1 --report
"""
from __future__ import annotations

import argparse
import html
import re
import sys
from html.parser import HTMLParser

# Elementos que encerram uma linha lógica. `br` também, porque o Planalto o usa
# como quebra de parágrafo dentro de células.
BLOCK_TAGS = {
    "p", "div", "br", "tr", "td", "th", "li", "h1", "h2", "h3", "h4", "h5", "h6",
    "table", "blockquote", "section", "article",
}
# Só tags com fechamento real podem entrar aqui: o skip é contado por
# profundidade, e um elemento vazio (`<meta>`, `<link>`) nunca dispara
# handle_endtag — deixaria o contador preso acima de zero e descartaria o
# documento inteiro em silêncio.
SKIP_TAGS = {"script", "style", "title"}
STRIKE_TAGS = {"strike", "s", "del"}


class PlanaltoExtractor(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.lines: list[str] = []
        self._buf: list[str] = []
        self._skip_depth = 0
        self._strike_depth = 0
        # Âncoras <a name="artN"> — marcador estrutural do Planalto, usado só
        # para o relatório de conferência (o corpus continua sendo ancorado
        # pelo texto "Art. N", que é o contrato do cmd/cleaner).
        self.anchor_names: list[str] = []
        self.strike_chars = 0

    def _flush(self) -> None:
        texto = "".join(self._buf)
        self._buf.clear()
        # Colapsa TODO espaço em branco interno (inclusive \n e \t) num espaço.
        texto = re.sub(r"\s+", " ", texto).strip()
        if texto:
            self.lines.append(texto)

    def handle_starttag(self, tag: str, attrs) -> None:
        if tag in SKIP_TAGS:
            self._skip_depth += 1
            return
        if tag in STRIKE_TAGS:
            self._strike_depth += 1
        if tag == "a":
            for k, v in attrs:
                if k == "name" and v:
                    self.anchor_names.append(v)
        if tag in BLOCK_TAGS:
            self._flush()

    def handle_endtag(self, tag: str) -> None:
        if tag in SKIP_TAGS:
            self._skip_depth = max(0, self._skip_depth - 1)
            return
        if tag in STRIKE_TAGS:
            self._strike_depth = max(0, self._strike_depth - 1)
        if tag in BLOCK_TAGS:
            self._flush()

    def handle_data(self, data: str) -> None:
        if self._skip_depth > 0:
            return
        if self._strike_depth > 0:
            self.strike_chars += len(data.strip())
        self._buf.append(data)

    def close(self) -> None:  # noqa: D102
        super().close()
        self._flush()


# "Art. 1º", "Art. 10", "Art. 1.º" — o mesmo dispositivo que o cleaner ancora.
RE_ARTIGO_INICIO = re.compile(r"^Art\.\s*\d+")
RE_ARTIGO_QUALQUER = re.compile(r"\bArt\.\s*\d+")
RE_ANCORA_ART = re.compile(r"^art\.?(\d+)$", re.IGNORECASE)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("-i", "--input", required=True, help="HTML baixado do Planalto")
    ap.add_argument("-o", "--output", required=True, help="texto de saída (UTF-8), entrada do cmd/cleaner")
    ap.add_argument(
        "--encoding",
        default="latin-1",
        help="encoding do HTML de origem (Planalto não declara charset; é ISO-8859-1)",
    )
    ap.add_argument("--report", action="store_true", help="imprime conferência estrutural no stderr")
    args = ap.parse_args()

    with open(args.input, "rb") as fh:
        raw = fh.read()
    try:
        texto_html = raw.decode(args.encoding)
    except UnicodeDecodeError as exc:
        print(f"erro: falha ao decodificar como {args.encoding}: {exc}", file=sys.stderr)
        return 1

    parser = PlanaltoExtractor()
    parser.feed(texto_html)
    parser.close()

    linhas = parser.lines
    with open(args.output, "w", encoding="utf-8", newline="\n") as fh:
        fh.write("\n".join(linhas) + "\n")

    if args.report:
        ancoras_art = [a for a in parser.anchor_names if RE_ANCORA_ART.match(a)]
        inicio = [ln for ln in linhas if RE_ARTIGO_INICIO.match(ln)]
        # Linhas que MENCIONAM um artigo mas não começam com ele: candidatas a
        # artigo que perderia a âncora no cleaner (a falha do "Art. 1º" da LC 68).
        meio = [ln for ln in linhas if RE_ARTIGO_QUALQUER.search(ln) and not RE_ARTIGO_INICIO.match(ln)]

        print(f"linhas emitidas............: {len(linhas)}", file=sys.stderr)
        print(f"âncoras <a name='artN'>....: {len(ancoras_art)} ({len(set(ancoras_art))} distintas)", file=sys.stderr)
        print(f"linhas começando com Art. N: {len(inicio)}  <- viram '#### Art. N' no cleaner", file=sys.stderr)
        print(f"linhas com Art. N no meio...: {len(meio)}  <- referências cruzadas + possíveis perdas", file=sys.stderr)
        print(f"caracteres em texto riscado.: {parser.strike_chars}  <- revogado no compilado; revisar", file=sys.stderr)

        faltantes = sorted(
            {int(RE_ANCORA_ART.match(a).group(1)) for a in ancoras_art}
            - {int(re.match(r"^Art\.\s*(\d+)", ln).group(1)) for ln in inicio}
        )
        if faltantes:
            print(
                f"ATENÇÃO: {len(faltantes)} artigos têm âncora no HTML mas NENHUMA linha começando por eles: "
                f"{faltantes[:20]}{'...' if len(faltantes) > 20 else ''}",
                file=sys.stderr,
            )
        else:
            print("todos os artigos ancorados no HTML começam uma linha ✓", file=sys.stderr)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
