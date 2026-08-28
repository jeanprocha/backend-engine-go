package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Arquivo-ouro do corpus legal (W1/Onda 2, PR 3).
//
// O Markdown limpo em docs/ é a ENTRADA da ingestão: cada `#### Art. N` vira um
// chunk, com o article_id que o dossiê depois cita. Se esse arquivo puder ser
// editado à mão sem ninguém perceber, a cadeia "texto oficial → chunk → citação"
// deixa de ser auditável — que é a única coisa que o corpus precisa garantir.
//
// Este teste fecha a cadeia: prova que o .md commitado é exatamente o que o
// cleaner commitado produz a partir do texto bruto commitado. Nenhuma etapa
// manual no meio.
//
// Regenerar (depois de baixar um texto novo ou mudar uma regra do profile):
//
//	go test ./cmd/cleaner/ -run TestGolden -cleaner-update
//
// e revisar o diff — ele é o entregável de auditoria, não um efeito colateral.
var cleanerUpdate = flag.Bool("cleaner-update", false, "regrava os Markdown limpos em docs/ a partir dos textos brutos")

type goldenDoc struct {
	nome    string
	entrada string // texto bruto extraído da fonte oficial
	saida   string // Markdown limpo, entrada da ingestão
	profile string
}

// LC 68/2024 fica DE FORA de propósito: o .md commitado foi produzido por uma
// versão anterior do cleaner e não é reproduzível byte a byte pela atual. Não é
// motivo para reescrevê-lo — o corpus ingerido em produção veio dele, e
// regravá-lo agora quebraria a correspondência com os 377 chunks que estão no
// banco. Entra aqui quando (e se) a LC 68 for re-ingerida.
var goldenDocs = []goldenDoc{
	{
		nome:    "LC 214/2025",
		entrada: "lc214-em-texto.txt",
		saida:   "docs/lc214_2025_limpa.md",
		profile: "planalto-dou",
	},
}

// normalizaEOL iguala CRLF a LF — ver o comentário no ponto de comparação.
func normalizaEOL(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

func repoRoot(t *testing.T) string {
	t.Helper()
	// Os testes rodam em cmd/cleaner/; os arquivos vivem na raiz do repo.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolver raiz do repo: %v", err)
	}
	return root
}

func TestGoldenCorpusEhReproduzivel(t *testing.T) {
	root := repoRoot(t)

	for _, doc := range goldenDocs {
		t.Run(doc.nome, func(t *testing.T) {
			profile, ok := Profiles()[doc.profile]
			if !ok {
				t.Fatalf("perfil %q desconhecido", doc.profile)
			}

			bruto, err := os.ReadFile(filepath.Join(root, doc.entrada))
			if err != nil {
				t.Fatalf("ler texto bruto %s: %v", doc.entrada, err)
			}

			gerado := Clean(string(bruto), profile)
			caminhoSaida := filepath.Join(root, doc.saida)

			if *cleanerUpdate {
				if err := os.WriteFile(caminhoSaida, []byte(gerado), 0o644); err != nil {
					t.Fatalf("gravar %s: %v", doc.saida, err)
				}
				t.Logf("regravado %s (%d bytes, %d âncoras de artigo)",
					doc.saida, len(gerado), strings.Count(gerado, "\n#### Art."))
				return
			}

			commitado, err := os.ReadFile(caminhoSaida)
			if err != nil {
				t.Fatalf("ler %s: %v (rode com -cleaner-update para gerar)", doc.saida, err)
			}

			// Compara CONTEÚDO, não bytes crus: no Windows o git converte LF em
			// CRLF no checkout, e o arquivo em disco fica maior que o que o
			// cleaner escreve (medido: 11.144 bytes de diferença, exatamente uma
			// quebra de linha a mais por linha). Sem normalizar, o teste falharia
			// só nesta plataforma, por um motivo que não tem nada a ver com o que
			// ele quer garantir.
			if normalizaEOL(string(commitado)) != normalizaEOL(gerado) {
				t.Errorf("%s divergiu do que o cleaner produz a partir de %s.\n"+
					"commitado: %d bytes, %d âncoras\ngerado:    %d bytes, %d âncoras\n"+
					"Se a mudança é intencional, rode: go test ./cmd/cleaner/ -run TestGolden -cleaner-update",
					doc.saida, doc.entrada,
					len(commitado), strings.Count(string(commitado), "\n#### Art."),
					len(gerado), strings.Count(gerado, "\n#### Art."))
			}
		})
	}
}
