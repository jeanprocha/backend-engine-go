package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Uso: go run ./cmd/cleaner [-in=lei-em-texto.txt] [-out=docs/lc68_2024_limpa.md] [-profile=camara-plp]
// Sem flags, reproduz o comportamento de sempre: lê lei-em-texto.txt (texto do
// PLP 68/2024, extraído do PDF da Câmara) e escreve docs/lc68_2024_limpa.md.
//
// -profile identifica a FONTE do texto bruto (ruído de cabeçalho/rodapé é
// diferente entre o PDF de tramitação da Câmara e o texto oficial do
// Planalto/DOU) — ver clean.go. Perfis disponíveis: camara-plp (default),
// planalto-dou, none.
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	defaultIn := "lei-em-texto.txt"
	defaultOut := "docs/lc68_2024_limpa.md"
	inPath := flag.String("in", defaultIn, "caminho do texto bruto (extraído de PDF/HTML)")
	outPath := flag.String("out", defaultOut, "caminho de saída do Markdown limpo")
	profileName := flag.String("profile", "camara-plp", "perfil de ruído da fonte: "+strings.Join(profileNames(), ", "))
	flag.Parse()

	profile, ok := Profiles()[*profileName]
	if !ok {
		return fmt.Errorf("perfil %q desconhecido; use um de: %s", *profileName, strings.Join(profileNames(), ", "))
	}

	content, err := os.ReadFile(*inPath)
	if err != nil {
		return fmt.Errorf("abrir %s: %w", *inPath, err)
	}

	cleaned := Clean(string(content), profile)

	if err := os.WriteFile(*outPath, []byte(cleaned), 0o644); err != nil {
		return fmt.Errorf("escrever %s: %w", *outPath, err)
	}

	fmt.Printf("Limpeza concluída (perfil %s)! Verifique %s\n", profile.Name, *outPath)
	fmt.Println("Re-ingestão: no Supabase, TRUNCATE TABLE public.tax_law_chunks; depois rode " +
		"cmd/ingest -file=" + *outPath + " -id-prefix=<prefixo> -source=\"<rótulo>\" " +
		"(ON CONFLICT não atualiza linhas antigas).")
	return nil
}

func profileNames() []string {
	names := make([]string, 0, len(Profiles()))
	for name := range Profiles() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
