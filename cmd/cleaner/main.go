package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	// 1. Abrir o arquivo bruto que você baixou
	content, err := os.ReadFile("lei-em-texto.txt")
	if err != nil {
		fmt.Printf("Erro ao abrir arquivo: %v\n", err)
		return
	}

	text := string(content)

	// 2. Limpeza bruta: cabeçalhos/rodapés de PDF (institucional, página, título repetido)
	reNoise := []*regexp.Regexp{
		// Letras espaçadas: "C Â M A R A  D O S  D E P U T A D O S"
		regexp.MustCompile(`(?i)C\s+[AÂ]\s*M\s+A\s+R\s+A\s+D\s+O\s+S\s+D\s+E\s+P\s+U\s+T\s+A\s+D\s+O\s+S`),
		// Forma compacta (linha inteira)
		regexp.MustCompile(`(?im)^.*[CÂ][AÂ]MARA\s+DOS\s+DEPUTADOS.*$`),
		// Título do projeto de lei (repetido no topo das páginas)
		regexp.MustCompile(`(?im)^.*PROJETO\s+DE\s+LEI\s+COMPLEMENTAR.*$`),
		// "Página X de Y", "Pág. X", variações
		regexp.MustCompile(`(?im)^.*[Pp][áa]g[ianção\.]*\s*\d+.*$`),
		// Linha contendo apenas número (ex.: número de página isolado)
		regexp.MustCompile(`(?m)^\s*\d+\s*$`),
		// Rodapé de apresentação: "Apresentação: DD/MM/AAAA HH:MM - PLP 68/2024"
		regexp.MustCompile(`(?i)Apresenta[cç][aã]o:.*?PLP\s*68/2024`),
		// Quebras de página (form feed)
		regexp.MustCompile(`\f`),
	}

	for _, re := range reNoise {
		text = re.ReplaceAllString(text, "")
	}

	// Colapsa múltiplos espaços/tabs em um espaço único (antes de remontar parágrafos)
	reMultiSpace := regexp.MustCompile(`[ \t]{2,}`)
	text = reMultiSpace.ReplaceAllString(text, " ")

	// 3. Reconstruir parágrafos quebrados
	// PDFs costumam quebrar linhas no meio da frase.
	// Vamos juntar linhas que não terminam com pontuação.
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	var currentLine strings.Builder
	reAbbrevDot := regexp.MustCompile(`(?i)\b(art|arts|n|nº|inc|par|pars)\.\s*$`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		currentLine.WriteString(trimmed)
		currentLine.WriteString(" ")

		// Não fechar parágrafo em abreviações jurídicas (ex.: "...de que trata o art." + linha "308 não poderá...")
		endsWithAbbrev := reAbbrevDot.MatchString(trimmed) ||
			strings.HasSuffix(trimmed, "º.") ||
			strings.HasSuffix(trimmed, "ª.")
		shouldFlush := !endsWithAbbrev &&
			(strings.HasSuffix(trimmed, ".") ||
				strings.HasSuffix(trimmed, ":") ||
				strings.HasSuffix(trimmed, ";"))

		if shouldFlush {
			cleanedLines = append(cleanedLines, currentLine.String())
			currentLine.Reset()
		}
	}

	// 4. Salvar como Markdown Estruturado (âncoras #### Art. N para o parser de ingest)
	outputFile, err := os.Create("docs/lc68_2024_limpa.md")
	if err != nil {
		fmt.Printf("Erro ao criar docs/lc68_2024_limpa.md: %v\n", err)
		return
	}
	defer outputFile.Close()
	writer := bufio.NewWriter(outputFile)

	reArtigo := regexp.MustCompile(`^(Art\.\s+\d+)`)

	for _, line := range cleanedLines {
		if reArtigo.MatchString(line) {
			writer.WriteString("\n\n#### " + line + "\n")
		} else {
			writer.WriteString(line + "\n")
		}
	}
	writer.Flush()
	fmt.Println("Limpeza concluída! Verifique docs/lc68_2024_limpa.md")
	fmt.Println("Re-ingestão: no Supabase, TRUNCATE TABLE public.tax_law_chunks; depois rode cmd/ingest (ON CONFLICT não atualiza linhas antigas).")
}
