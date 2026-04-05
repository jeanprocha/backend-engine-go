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
// Usa o padrão "#### Art. Nº" produzido pelo cmd/cleaner como âncora,
// evitando falsos positivos com referências inline como "nos termos do art. 5º".
func (p *Parser) ParseArticles() []ArticleChunk {
	re := regexp.MustCompile(`(?m)^####\s+(Art\.\s+\d+[º°]?\.?)`)
	reHeader := regexp.MustCompile(`^####\s+`)

	idxs := re.FindAllStringSubmatchIndex(p.rawText, -1)
	var result []ArticleChunk

	for i, sm := range idxs {
		titleStart, titleEnd := sm[2], sm[3]
		contentStart := sm[0]
		contentEnd := len(p.rawText)
		if i+1 < len(idxs) {
			contentEnd = idxs[i+1][0]
		}

		title := strings.TrimSpace(p.rawText[titleStart:titleEnd])
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

// splitBySeparator divide o conteúdo pelo separador e agrupa em blocos
// respeitando maxChunkChars. Retorna os blocos mesmo que algum ainda seja grande.
func splitBySeparator(title, content, sep string) []string {
	rawParts := strings.Split(content, sep)

	var parts []string
	var current strings.Builder
	current.WriteString(title + "\n")

	for _, para := range rawParts {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		if current.Len()+len(para)+2 > maxChunkChars && current.Len() > len(title)+1 {
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
			current.WriteString(title + " (continuação)\n")
		}

		current.WriteString(para)
		current.WriteString(sep)
	}

	if s := strings.TrimSpace(current.String()); s != "" && s != strings.TrimSpace(title) {
		parts = append(parts, s)
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

// splitBySize é o último recurso: corte duro com prefixo de título em cada parte.
// Recua até encontrar espaço ou quebra para não cortar no meio de uma palavra.
func splitBySize(title, text string, maxSize int) []string {
	var parts []string
	prefix := title + "\n"
	// Desconta o prefixo do espaço disponível em cada parte.
	available := maxSize - len(prefix)
	if available <= 0 {
		available = maxSize
		prefix = ""
	}

	for len(text) > available {
		cut := available
		// Recua até espaço ou quebra de linha para não cortar palavra no meio.
		for cut > 0 && text[cut] != ' ' && text[cut] != '\n' {
			cut--
		}
		if cut == 0 {
			cut = available
		}
		parts = append(parts, strings.TrimSpace(prefix+text[:cut]))
		text = strings.TrimSpace(text[cut:])
		// A partir da segunda parte, indica continuação no prefixo.
		prefix = title + " (continuação)\n"
	}

	if text != "" {
		parts = append(parts, strings.TrimSpace(prefix+text))
	}
	return parts
}

// sanitizeID converte "Art. 1º" em "art_1" para uso seguro como identificador.
func sanitizeID(title string) string {
	s := strings.ToLower(title)
	s = regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(s, "_")
	s = regexp.MustCompile(`_+`).ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}
