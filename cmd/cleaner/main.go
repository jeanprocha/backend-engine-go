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

	cleaned, report := CleanWithReport(string(content), profile)

	if err := os.WriteFile(*outPath, []byte(cleaned), 0o644); err != nil {
		return fmt.Errorf("escrever %s: %w", *outPath, err)
	}

	fmt.Printf("Limpeza concluída (perfil %s)! Verifique %s\n", profile.Name, *outPath)

	// Nada que muda o CONTEÚDO do corpus pode passar despercebido: quem roda
	// precisa ver o que ficou de fora do texto oficial, e quanto.
	if profile.CorpusStopAtRe != nil {
		if n := descartadoPeloCorte(string(content), profile); n > 0 {
			fmt.Printf("ESCOPO: %d caracteres do fim do documento ficaram FORA do corpus "+
				"(corte em %q — ver Profile.CorpusStopAtRe). O texto bruto de entrada continua completo.\n",
				n, profile.CorpusStopAtRe.String())
		}
	}
	if n := len(report.ArtigosSuperadosRemovidos); n > 0 {
		fmt.Printf("REDAÇÃO SUPERADA: %d artigos tinham duas redações no texto compilado; "+
			"ficou só a vigente (a que traz a marca de redação dada por lei posterior): %s\n",
			n, strings.Join(report.ArtigosSuperadosRemovidos, ", "))
	}
	if n := len(report.ArtigosDuplicadosMantidos); n > 0 {
		fmt.Printf("REVISAR: %d artigos aparecem mais de uma vez mas a última ocorrência NÃO "+
			"traz marca de redação vigente — mantidos TODOS, para não apagar texto legal por "+
			"suposição. Conferir manualmente: %s\n",
			n, strings.Join(report.ArtigosDuplicadosMantidos, ", "))
	}
	fmt.Println("Re-ingestão: no Supabase, TRUNCATE TABLE public.tax_law_chunks; depois rode " +
		"cmd/ingest -file=" + *outPath + " -id-prefix=<prefixo> -source=\"<rótulo>\" " +
		"(ON CONFLICT não atualiza linhas antigas).")
	return nil
}

// descartadoPeloCorte mede quantos caracteres do texto BRUTO ficaram fora do
// corpus por causa de CorpusStopAtRe — serve só para o aviso ao operador.
//
// Varre LINHA A LINHA, como Clean faz: o regex é ancorado em "^" sem a flag
// multilinha, então avaliá-lo contra o texto inteiro casaria apenas no offset 0.
func descartadoPeloCorte(bruto string, p Profile) int {
	offset := 0
	for _, line := range strings.Split(bruto, "\n") {
		if p.CorpusStopAtRe.MatchString(strings.TrimSpace(line)) {
			return len(bruto) - offset
		}
		offset += len(line) + 1 // +1 = o \n consumido pelo Split
	}
	return 0
}

func profileNames() []string {
	names := make([]string, 0, len(Profiles()))
	for name := range Profiles() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
