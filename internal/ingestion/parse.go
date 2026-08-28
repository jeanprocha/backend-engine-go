package ingestion

import (
	"fmt"
	"regexp"
	"strings"
)

// ArticleChunk representa um fragmento da lei pronto para virar um embedding.
// Artigos muito longos são divididos em partes, mantendo o article_id original
// nos metadados para rastreabilidade.
type ArticleChunk struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
}

// maxChunkChars é o limite seguro de caracteres por chunk.
// Texto legal brasileiro é denso em tokens: listas de incisos, siglas e números
// elevam a proporção tokens/char bem acima da média de texto corrido.
// 4000 chars garante margem segura mesmo nos artigos mais densos.
const maxChunkChars = 4000

// DocumentProfile identifica QUAL documento normativo está a ser ingerido.
// O prefixo entra no article_id de cada chunk, que é a chave única global da
// tabela tax_law_chunks — é ele que permite dois documentos coexistirem no
// mesmo corpus sem colisão (ex.: o "Art. 1" da LC 214 e o da LC 227 viram
// lc214_0001_art_1 e lc227_0001_art_1).
type DocumentProfile struct {
	// IDPrefix prefixa o article_id. Deve terminar em "_" (ex.: "lc214_").
	IDPrefix string
	// SourceLabel vai para metadata["source"] e é o rótulo exibido ao usuário.
	SourceLabel string
}

// DefaultDocumentProfile descreve o documento QUE ESTÁ NO BANCO hoje — não o
// que se deseja ter. Mudar estes valores sem re-ingerir órfã os article_id
// já persistidos (âncoras de dossiês salvos deixam de resolver). A troca para
// a LC 214/2025 acontece por flag no cmd/ingest, junto da re-ingestão.
func DefaultDocumentProfile() DocumentProfile {
	return DocumentProfile{IDPrefix: "lc68_", SourceLabel: "LC 68/2024"}
}

var reIDPrefix = regexp.MustCompile(`^[a-z0-9]+_$`)

// Validate rejeita perfis malformados. Chamado antes de gastar embeddings:
// um typo no prefixo só apareceria depois de pagar a API inteira.
func (d DocumentProfile) Validate() error {
	if !reIDPrefix.MatchString(d.IDPrefix) {
		return fmt.Errorf("id-prefix %q inválido: use minúsculas/dígitos terminando em _ (ex.: lc214_)", d.IDPrefix)
	}
	if strings.TrimSpace(d.SourceLabel) == "" {
		return fmt.Errorf("source não pode ser vazio")
	}
	return nil
}

// Parser é o motor que entende a estrutura da lei.
type Parser struct {
	rawText string
	doc     DocumentProfile
}

// NewParser usa o documento default (o que está ingerido hoje).
func NewParser(text string) *Parser {
	return NewParserForDocument(text, DefaultDocumentProfile())
}

// NewParserForDocument parseia identificando explicitamente o documento.
func NewParserForDocument(text string, doc DocumentProfile) *Parser {
	return &Parser{rawText: text, doc: doc}
}

// stripTitlePrefixForPath remove a primeira linha de continuação do artigo para análise
// de estrutura (parágrafo/inciso), evitando ruído do prefixo repetido em partes longas.
func stripTitlePrefixForPath(part, title string) string {
	s := strings.TrimSpace(part)
	lines := strings.SplitN(s, "\n", 2)
	if len(lines) < 2 {
		return s
	}
	first := strings.TrimSpace(lines[0])
	t := strings.TrimSpace(title)
	if strings.HasPrefix(first, t) || strings.Contains(first, "(continuação)") {
		return strings.TrimSpace(lines[1])
	}
	return s
}

// ParseArticles fatia a lei por artigos e aplica sub-chunking nos artigos
// que excedam maxChunkChars.
//
// Usa cabeçalho Markdown no início de linha: um ou mais #, espaços opcionais e "Art."
// seguido do número (e opcionalmente º/°, sufixo de letra e ponto). O match para no fim da
// referência do artigo; o texto do caput na mesma linha permanece em raw e, após remover só o
// #..., integra o Content.
// (?m)^ e "Art." com A maiúsculo evitam referências inline (ex.: "nos termos do art. 5º").
//
// O sufixo de letra ((-[A-Z]+)?) captura artigos INSERIDOS por lei posterior —
// "Art. 323-A", "Art. 7º-A". Sem ele o título sai como "Art. 323" e dois
// dispositivos distintos passam a reivindicar a mesma citação: o texto
// consolidado da LC 214/2025 tem 544 artigos-base e 36 variantes com letra
// (levantamento da Onda 2/PR 3), das quais 32 colidiriam com um artigo real.
// Para um produto cuja tese é citação auditável, isso é desqualificante.
// A LC 68/2024 não tem nenhuma âncora com letra (só menções inline a outras
// leis que ela altera), então esta mudança é inócua para o corpus ingerido hoje
// — travado por teste.
func (p *Parser) ParseArticles() []ArticleChunk {
	re := regexp.MustCompile(`(?m)^#+\s*Art\.\s*(\d+)([º°]?)(-[A-Z]+)?(\.)?`)
	reHeader := regexp.MustCompile(`^#+\s*`)

	idxs := re.FindAllStringSubmatchIndex(p.rawText, -1)
	var result []ArticleChunk

	for i, sm := range idxs {
		contentStart := sm[0]
		contentEnd := len(p.rawText)
		if i+1 < len(idxs) {
			contentEnd = idxs[i+1][0]
		}

		title := articleTitleFromSubmatch(p.rawText, sm)
		raw := strings.TrimSpace(p.rawText[contentStart:contentEnd])
		content := strings.TrimSpace(reHeader.ReplaceAllString(raw, ""))

		// Índice sequencial global (antes de expandir partes) para o ID base.
		seqBase := i + 1

		if len(content) > maxChunkChars {
			fmt.Printf("  artigo longo (%d chars): %s — dividindo...\n", len(content), title)
		}

		if len(content) <= maxChunkChars {
			meta := map[string]string{
				"source":     p.doc.SourceLabel,
				"type":       "article",
				"article_id": title,
			}
			path := AnalyzeLegalPath(title, content)
			ApplyLegalPathToMetadata(meta, path)
			result = append(result, ArticleChunk{
				ID:       fmt.Sprintf("%s%04d_%s", p.doc.IDPrefix, seqBase, sanitizeID(title)),
				Title:    title,
				Content:  content,
				Metadata: meta,
			})
			continue
		}

		// Artigo longo: divide em partes por parágrafo, preservando contexto.
		parts := splitLongContent(title, content)
		for j, part := range parts {
			meta := map[string]string{
				"source":      p.doc.SourceLabel,
				"type":        "article_part",
				"article_id":  title,
				"part":        fmt.Sprintf("%d", j+1),
				"total_parts": fmt.Sprintf("%d", len(parts)),
			}
			path := AnalyzeLegalPath(title, stripTitlePrefixForPath(part, title))
			ApplyLegalPathToMetadata(meta, path)
			result = append(result, ArticleChunk{
				ID:       fmt.Sprintf("%s%04d_%s_p%d", p.doc.IDPrefix, seqBase, sanitizeID(title), j+1),
				Title:    fmt.Sprintf("%s (parte %d de %d)", title, j+1, len(parts)),
				Content:  part,
				Metadata: meta,
			})
		}
	}

	return result
}

// articleTitleFromSubmatch monta o título canónico (ex.: "Art. 52.", "Art. 2º",
// "Art. 323-A.", "Art. 7º-A") a partir dos grupos (\d+)([º°]?)(-[A-Z]+)?(\.)?
// do regex de âncora — na ordem em que aparecem no texto legal.
func articleTitleFromSubmatch(s string, sm []int) string {
	if len(sm) < 10 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Art. ")
	b.WriteString(s[sm[2]:sm[3]]) // número
	// Grupos opcionais: índice negativo (sm[i] == -1) quando não participaram
	// do match, caso em que sm[i] < sm[i+1] é falso e o trecho é pulado.
	for _, g := range [][2]int{{sm[4], sm[5]}, {sm[6], sm[7]}, {sm[8], sm[9]}} {
		if g[0] >= 0 && g[0] < g[1] {
			b.WriteString(s[g[0]:g[1]])
		}
	}
	return b.String()
}

// splitLongContent divide o conteúdo de um artigo longo em partes coesas.
//
// Estratégia de divisão (ordem de preferência):
//  1. Por parágrafo ("§ N" ou linha em branco dupla)
//  2. Por ponto final se os parágrafos ainda forem grandes demais
//
// O título do artigo é prefixado em cada parte para que o RAG entenda
// o contexto mesmo recebendo apenas uma fatia.
func splitLongContent(title, content string) []string {
	// Tenta primeiro quebrar por parágrafo duplo (\n\n).
	// Se o artigo não tiver \n\n (bloco contínuo), tenta \n simples.
	// Se ainda assim um bloco individual exceder o limite, aplica corte por tamanho.
	separators := []string{"\n\n", "\n"}

	for _, sep := range separators {
		result := splitBySeparator(title, content, sep)
		// Só aceita o resultado se todos os fragmentos estiverem dentro do limite.
		if allWithinLimit(result) {
			return result
		}
	}

	// Último recurso: corte duro por tamanho, sem perder dados.
	return splitBySize(title, content, maxChunkChars)
}

// splitBySeparator divide o conteúdo pelo separador e agrupa em blocos.
// O corpo acumulado não inclui o prefixo; o orçamento por chunk é maxChunkChars menos len(prefix),
// para que prefix+corpo nunca ultrapasse o teto. Parágrafos maiores que o orçamento são fatiados
// via splitWithPrefixMax (prefixo de continuação correto após o primeiro flush).
func splitBySeparator(title, content, sep string) []string {
	rawParts := strings.Split(content, sep)
	firstPrefix := title + "\n"
	continuationPrefix := title + " (continuação)\n"

	var parts []string
	var body strings.Builder
	currentPrefix := firstPrefix
	bodyLimit := maxChunkChars - len(currentPrefix)
	if bodyLimit < 0 {
		bodyLimit = 0
	}

	for _, para := range rawParts {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		addLen := len(para) + len(sep)

		if body.Len() > 0 && body.Len()+addLen > bodyLimit {
			parts = append(parts, strings.TrimSpace(currentPrefix+body.String()))
			body.Reset()
			currentPrefix = continuationPrefix
			bodyLimit = maxChunkChars - len(currentPrefix)
			if bodyLimit < 0 {
				bodyLimit = 0
			}
		}

		if len(para) > bodyLimit {
			if body.Len() > 0 {
				parts = append(parts, strings.TrimSpace(currentPrefix+body.String()))
				body.Reset()
				currentPrefix = continuationPrefix
				bodyLimit = maxChunkChars - len(currentPrefix)
				if bodyLimit < 0 {
					bodyLimit = 0
				}
			}
			sub := splitWithPrefixMax(currentPrefix, continuationPrefix, para, maxChunkChars)
			parts = append(parts, sub...)
			currentPrefix = continuationPrefix
			bodyLimit = maxChunkChars - len(currentPrefix)
			if bodyLimit < 0 {
				bodyLimit = 0
			}
			continue
		}

		body.WriteString(para)
		body.WriteString(sep)
	}

	if body.Len() > 0 {
		parts = append(parts, strings.TrimSpace(currentPrefix+body.String()))
	}

	return parts
}

// allWithinLimit retorna true se todos os fragmentos respeitam maxChunkChars.
func allWithinLimit(parts []string) bool {
	for _, p := range parts {
		if len(p) > maxChunkChars {
			return false
		}
	}
	return len(parts) > 0
}

// splitBySize é o último recurso: corte duro com prefixo de título na primeira peça
// e "(continuação)" nas seguintes.
func splitBySize(title, text string, maxSize int) []string {
	first := title + "\n"
	cont := title + " (continuação)\n"
	return splitWithPrefixMax(first, cont, text, maxSize)
}

// splitWithPrefixMax fatia text em segmentos; cada saída é prefix+trecho com len <= maxTotal.
// A primeira peça usa firstPrefix; as demais, continuationPrefix.
// Se o prefixo exceder maxTotal (título patológico), o prefixo é omitido nessa fatia.
// Índices são em bytes (mesmo contrato que o restante do pacote).
func splitWithPrefixMax(firstPrefix, continuationPrefix, text string, maxTotal int) []string {
	if maxTotal <= 0 {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	prefix := firstPrefix
	for {
		if len(prefix) >= maxTotal {
			prefix = ""
		}
		avail := maxTotal - len(prefix)
		if avail <= 0 {
			avail = maxTotal
			prefix = ""
		}
		if len(text) <= avail {
			out = append(out, strings.TrimSpace(prefix+text))
			break
		}
		cut := avail
		for cut > 0 && text[cut] != ' ' && text[cut] != '\n' {
			cut--
		}
		if cut == 0 {
			cut = avail
		}
		out = append(out, strings.TrimSpace(prefix+text[:cut]))
		text = strings.TrimSpace(text[cut:])
		prefix = continuationPrefix
	}
	return out
}

// sanitizeID converte "Art. 1º" em "art_1" para uso seguro como identificador.
func sanitizeID(title string) string {
	s := strings.ToLower(title)
	s = regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(s, "_")
	s = regexp.MustCompile(`_+`).ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}
