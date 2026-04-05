package classifier

import (
	"fmt"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/ingestion"
)

// systemPrompt é o manual de instruções da LLM.
// Restrições explícitas evitam alucinações: a IA só pode usar o contexto fornecido
// e deve retornar um JSON puro sem texto extra (facilita o Unmarshal).
const systemPrompt = `Você é um Especialista em Direito Tributário Brasileiro focado na Lei Complementar 68/2024 (Reforma Tributária - CBS/IBS).

Sua tarefa é analisar se uma despesa descrita pelo usuário gera direito a crédito de IBS e CBS para o contribuinte.

REGRAS OBRIGATÓRIAS:
1. Baseie-se EXCLUSIVAMENTE nos artigos da lei fornecidos no contexto abaixo.
2. Se os artigos não cobrirem o tipo de despesa, responda com is_eligible: false, confidence: 0.0 e risk_level: "alto".
3. NUNCA invente regras ou use conhecimento externo à lei fornecida.
4. A regra geral da LC 68/2024 é não-cumulatividade plena: crédito é permitido se o bem/serviço for usado na atividade econômica, EXCETO uso/consumo pessoal.
5. Responda APENAS com JSON puro, sem markdown, sem texto antes ou depois do bloco JSON.
6. Quando a CATEGORIA JURÍDICA SUGERIDA for fornecida, use-a como interpretação canônica da despesa para aplicar as regras do corpo da lei. Não exija que o nome original da despesa apareça literalmente nos artigos.
7. Priorize regras gerais do corpo da lei (Arts. 1–400) sobre listas de Anexos. Anexos descrevem produtos específicos (agrícolas, culturais, médicos); se o item não for literalmente um produto de Anexo, aplique a regra geral do Art. 28.

SCHEMA DE RESPOSTA (sem desvios):
{"is_eligible":bool,"confidence":float,"justification":"string curta e técnica","legal_base":"Art. X, inciso Y","risk_level":"baixo|medio|alto"}`

// buildUserMessage monta a mensagem do usuário com contexto jurídico + pergunta.
// Os artigos são numerados para que a LLM possa referenciá-los na justificativa.
// legalCategory é a tradução jurídica gerada pelo expandQuery (ex: "licenciamento de
// software, bens imateriais"); quando fornecida, é exibida como "CATEGORIA JURÍDICA
// SUGERIDA" para que a LLM não exija que o nome coloquial apareça literalmente na lei.
func buildUserMessage(description, legalCategory, companyContext string, articles []ingestion.SearchResult) string {
	var sb strings.Builder

	sb.WriteString("CONTEXTO JURÍDICO (artigos recuperados da LC 68/2024):\n\n")
	for i, a := range articles {
		sb.WriteString(fmt.Sprintf("[%d] %s (similaridade: %.2f)\n%s\n\n",
			i+1, a.ArticleID, a.Similarity, a.Content))
	}

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("DESPESA A CLASSIFICAR: %q\n", description))

	if legalCategory != "" && legalCategory != description {
		sb.WriteString(fmt.Sprintf("CATEGORIA JURÍDICA SUGERIDA: %s\n", legalCategory))
	}

	if companyContext != "" {
		sb.WriteString(fmt.Sprintf("CONTEXTO DA EMPRESA: %s\n", companyContext))
	}

	sb.WriteString("\nCom base EXCLUSIVAMENTE nos artigos acima, esta despesa gera crédito de IBS/CBS?")

	return sb.String()
}
