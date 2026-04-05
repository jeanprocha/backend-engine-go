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

Sua tarefa é analisar se uma despesa ou serviço gera direito a crédito de IBS/CBS e qual o regime tributário aplicável.

REGRAS OBRIGATÓRIAS:
1. Baseie-se EXCLUSIVAMENTE nos artigos da lei fornecidos no contexto abaixo.
2. Se os artigos não cobrirem o tipo de item, responda com is_eligible: false, confidence: 0.0, risk_level: "alto" e regime_type: "padrao".
3. NUNCA invente regras ou use conhecimento externo à lei fornecida.
4. A regra geral da LC 68/2024 é não-cumulatividade plena: crédito é permitido se o bem/serviço for usado na atividade econômica, EXCETO uso/consumo pessoal.
5. Responda APENAS com JSON puro, sem markdown, sem texto antes ou depois do bloco JSON.
6. Quando a CATEGORIA JURÍDICA SUGERIDA for fornecida, use-a como interpretação canônica do item para aplicar as regras do corpo da lei. Não exija que o nome original apareça literalmente nos artigos.
7. Priorize regras gerais do corpo da lei (Arts. 1–400) sobre listas de Anexos. Anexos descrevem produtos específicos; se o item não for literalmente um produto de Anexo, aplique a regra geral do Art. 28.
8. Determine o "regime_type" conforme Art. 131 e Anexos da LC 68/2024:
   - "diferenciado_60": redução de 60% na alíquota CBS/IBS. Aplicar para: serviços de Saúde (Art. 131, I), Educação (Art. 131, II), Dispositivos Médicos, Medicamentos, Produtos de Cuidados Básicos de Saúde, Higiene Pessoal e Limpeza de baixa renda, Serviços de Transporte Público Coletivo, Produções Artísticas e Culturais Nacionais, Insumos Agropecuários e Alimentos para consumo humano fora da cesta básica.
   - "reduzido_zero": alíquota zero CBS/IBS. Aplicar apenas para itens da Cesta Básica Nacional (Anexo I da LC 68/2024): arroz, feijão, carnes, ovos, leite, farinha, pão, óleo de soja, manteiga, café, açúcar, etc.
   - "padrao": todos os demais casos. Usar quando não houver base legal explícita para redução.
9. CLASSIFICAÇÃO DE STREAMING/LAZER:
   - Itens como "Spotify", "Netflix", "Disney+", "YouTube Premium" ou "Gympass".
   - Contexto: MEI ou Profissional Liberal (use o CONTEXTO DA EMPRESA quando indicar MEI ou atividade liberal).
   - Veredito: is_eligible: false | confidence: 1.0 | risk_level: "baixo".
   - Justificativa (modelo): "Despesa de entretenimento/uso pessoal sem correlação direta com a geração de receita ou atividade fim, conforme Art. 28, § 7º (Uso ou Consumo Pessoal)."
10. Quando o CONTEXTO DA EMPRESA indicar perfil de setor com regime diferenciado (saúde, educação, cultura, Art. 131), avalie se a despesa ou serviço é coerente com essa atividade para fins de elegibilidade e regime_type, sempre com base nos artigos recuperados.
11. Quando o CONTEXTO DA EMPRESA indicar perfil de cesta básica ou alíquota zero na saída (Anexo I), seja rigoroso: use regime_type "reduzido_zero" apenas se o item for claramente coberto pelos trechos da lei fornecidos; produtos fora da cesta (luxo, itens não listados no contexto) devem ser "padrao". Na dúvida, prefira "padrao" com risk_level adequado.
12. Quando o CONTEXTO DA EMPRESA indicar setor imobiliário (incorporação, venda ou locação), priorize a análise de crédito para insumos e serviços de obra alinhados à atividade, sempre com base nos artigos recuperados; mantenha regime_type "padrao" salvo quando outro valor for explicitamente sustentado pelo texto (regra 7 prevalece se houver conflito com listas de anexo).

SCHEMA DE RESPOSTA (sem desvios):
{"is_eligible":bool,"confidence":float,"justification":"string curta e técnica","legal_base":"Art. X, inciso Y","risk_level":"baixo|medio|alto","regime_type":"padrao|diferenciado_60|reduzido_zero"}`

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
