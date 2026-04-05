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

// Parser é o motor que entende a estrutura da lei.
type Parser struct {
	rawText string
}

func NewParser(text string) *Parser {
	return &Parser{rawText: text}
}

// ParseArticles fatia a lei por artigos e aplica sub-chunking nos artigos
// que excedam maxChunkChars.
//
// Usa cabeçalho Markdown no início de linha: um ou mais #, espaços opcionais e "Art."
// seguido do número (e opcionalmente º/° ou ponto). O match para no fim da referência do artigo;
// o texto do caput na mesma linha permanece em raw e, após remover só o #..., integra o Content.
// (?m)^ e "Art." com A maiúsculo evitam referências inline (ex.: "nos termos do art. 5º").
func (p *Parser) ParseArticles() []ArticleChunk {
	re := regexp.MustCompile(`(?m)^#+\s*Art\.\s*(\d+)([º°]?)(\.)?`)
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
			result = append(result, ArticleChunk{
				ID:    fmt.Sprintf("lc68_%04d_%s", seqBase, sanitizeID(title)),
				Title: title,
				Content: content,
				Metadata: map[string]string{
					"source":     "LC 68/2024",
					"type":       "article",
					"article_id": title,
				},
			})
			continue
		}

		// Artigo longo: divide em partes por parágrafo, preservando contexto.
		parts := splitLongContent(title, content)
		for j, part := range parts {
			result = append(result, ArticleChunk{
				ID:    fmt.Sprintf("lc68_%04d_%s_p%d", seqBase, sanitizeID(title), j+1),
				Title: fmt.Sprintf("%s (parte %d de %d)", title, j+1, len(parts)),
				Content: part,
				Metadata: map[string]string{
					"source":     "LC 68/2024",
					"type":       "article_part",
					"article_id": title,
					"part":       fmt.Sprintf("%d", j+1),
					"total_parts": fmt.Sprintf("%d", len(parts)),
				},
			})
		}
	}

	return result
}

// articleTitleFromSubmatch monta o título canónico (ex.: "Art. 52.", "Art. 2º") a partir
// dos grupos (\d+)([º°]?)(\.)? do regex de âncora.
func articleTitleFromSubmatch(s string, sm []int) string {
	if len(sm) < 8 {
		return ""
	}
	num := s[sm[2]:sm[3]]
	var b strings.Builder
	b.WriteString("Art. ")
	b.WriteString(num)
	if sm[4] < sm[5] {
		b.WriteString(s[sm[4]:sm[5]])
	}
	if sm[6] < sm[7] {
		b.WriteString(s[sm[6]:sm[7]])
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
