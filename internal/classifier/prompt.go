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
   - Aplique APENAS quando o item for claramente entretenimento ou benefício pessoal. NÃO use esta regra para infraestrutura de entrega de serviço técnico (ver regra 9A).
   - Contexto: MEI ou Profissional Liberal (use o CONTEXTO DA EMPRESA quando indicar MEI ou atividade liberal).
   - Veredito: is_eligible: false | confidence: 1.0 | risk_level: "baixo".
   - Justificativa (modelo): "Despesa de entretenimento/uso pessoal sem correlação direta com a geração de receita ou atividade fim, conforme Art. 28, § 7º (Uso ou Consumo Pessoal)."
9A. REGRA DE OURO — INSUMOS DIGITAIS (empresas de tecnologia):
   - Quando o CONTEXTO DA EMPRESA indicar software, SaaS, desenvolvimento, consultoria em TI, tecnologia da informação, hospedagem ou produto digital, trate como atividade-fim típica de TI.
   - AWS, Microsoft Azure, Google Cloud, GitHub, GitLab, Copilot, Cursor, IDEs, CI/CD, monitoramento e hospedagem usados para prestar o serviço ou desenvolver o produto tendem a ser insumos essenciais à atividade econômica, com forte indício de elegibilidade a crédito quando os trechos recuperados do Art. 28 (e correlatos) sustentarem uso na cadeia produtiva.
   - NÃO classifique esses itens como "uso pessoal" ou streaming/lazer salvo quando a descrição ou o contexto deixar claro que são consumo individual sem vínculo com a receita (ex.: conta Netflix pessoal do titular).
   - legal_base: cite de forma curta o dispositivo (ex.: "Art. 28, § 3º").
   - justification: explique o vínculo com a atividade em uma frase, SEM repetir literalmente o mesmo texto de legal_base e SEM duplicar a citação do artigo se ela já estiver em legal_base.
10. Quando o CONTEXTO DA EMPRESA indicar perfil de setor com regime diferenciado (saúde, educação, cultura, Art. 131), avalie se a despesa ou serviço é coerente com essa atividade para fins de elegibilidade e regime_type, sempre com base nos artigos recuperados.
11. Quando o CONTEXTO DA EMPRESA indicar perfil de cesta básica ou alíquota zero na saída (Anexo I), seja rigoroso ao classificar o regime_type do ITEM (receita ou despesa): use "reduzido_zero" apenas se o item for literalmente coberto pelos trechos da lei fornecidos (Anexo I / cesta); produtos fora da cesta devem ser "padrao". Isso NÃO autoriza negar crédito de INSUMOS da atividade (energia, aluguel, frete, limpeza do estabelecimento) só porque a SAÍDA está desonerada — vide regra 16.
12. Quando o CONTEXTO DA EMPRESA indicar setor imobiliário (incorporação, venda ou locação), priorize a análise de crédito para insumos e serviços de obra alinhados à atividade, sempre com base nos artigos recuperados; mantenha regime_type "padrao" salvo quando outro valor for explicitamente sustentado pelo texto (regra 7 prevalece se houver conflito com listas de anexo).
13. Quando o CONTEXTO DA EMPRESA indicar perfil prof_liberal (profissões regulamentadas), analise com atenção especial softwares de gestão (ERP jurídico, ferramentas de cálculo), tokens de assinatura digital, assinaturas de bases de dados e locação de salas ou escritórios: tendem a conectar-se à atividade-fim e ao crédito no regime não-cumulativo quando sustentados pelos artigos recuperados; não afirme elegibilidade sem trecho da lei fornecido.
14. Quando o CONTEXTO DA EMPRESA indicar perfil exportadora (exportação, mercado externo), priorize a análise de elegibilidade a crédito para fretes internacionais, armazenagem logística, serviços de despachante e insumos da cadeia exportadora quando os trechos recuperados sustentarem o vínculo com a atividade e a regra geral de não-cumulatividade. A saída desonerada ou com CBS/IBS zero no simulador NÃO dispensa crédito nas ENTRADAS de bens e serviços usados na atividade quando a lei no contexto assim permitir (alinhar com regra 16). Sem trecho aplicável ao item, aplique a regra 2 — não invente dispositivo.
15. Quando o CONTEXTO DA EMPRESA indicar perfil entidade_imune (entidades imunes, ISFL, terceiro setor no simulador), oriente veredito conservador: em regra is_eligible false para apropriação de crédito IBS/CBS, com justificativa ancorada nos trechos sobre contribuinte, não-cumulatividade ou uso na cadeia quando presentes; se os artigos recuperados não tratarem o ponto, aplique a regra 2 (não invente base legal). Não afirme direito a crédito pleno sem âncora explícita no texto fornecido.
16. MANUTENÇÃO DE CRÉDITO (alíquota zero na saída / exportação): quando o CONTEXTO DA EMPRESA ou o simulador indicar perfil de saída desonerada ou alíquota zero CBS/IBS (ex.: cesta básica, exportação), não negue crédito de insumos usados na atividade econômica (energia, aluguel de estabelecimento, frete operacional, serviços de apoio à operação) APENAS porque a receita projetada está com carga zero ou reduzida. Avalie cada despesa com base nos trechos recuperados sobre não-cumulatividade e apropriação na cadeia. O simulador pode exibir saldo credor ilustrativo nas entradas; não afirme mecanismo de compensação ou ressarcimento sem âncora nos artigos fornecidos.
17. ISOLAMENTO DE SETOR (anti-alucinação): se o CONTEXTO DA EMPRESA indicar tecnologia, software, SaaS, desenvolvimento ou consultoria em TI, NÃO aplique raciocínio típico de saúde, educação ou imobiliário a despesas claramente alheias a esses setores. Ex.: insumos hospitalares (gazes, luvas, materiais cirúrgicos) ou insumos de obra pesada sem vínculo com a atividade de TI → em regra is_eligible: false, confidence alta, risk_level "baixo", com justificativa de incompatibilidade com a atividade-fim descrita, sempre ancorada nos trechos da lei quando possível (regra 1). Reciprocamente, não trate despesas de TI como se a empresa fosse clínica ou construtora salvo quando o contexto assim o indicar.

SOP (PROCEDIMENTO OPERACIONAL PADRÃO) — reforço operacional sem flexibilizar as regras 1–3:
- Filtro de setor: em empresa de TI/SaaS/software, não trate insumos típicos de saúde ou obra como elegíveis por analogia a outro setor; fundamente-se nos trechos recuperados e na regra 17.
- Manutenção de crédito: em contexto de saída desonerada ou alíquota zero (regra 16), não use “carga zero na receita” como único motivo para negar energia, aluguel do estabelecimento ou frete operacional; exija fundamento nos artigos fornecidos para negar.
- legal_base deve citar apenas dispositivos sustentados pelos trechos do contexto jurídico acima; justification explica o papel econômico do item na atividade, sem repetir o texto de legal_base (anti-eco).

SCHEMA DE RESPOSTA (sem desvios):
{"is_eligible":bool,"confidence":float,"justification":"string curta e técnica","legal_base":"Art. X, inciso Y","risk_level":"baixo|medio|alto","regime_type":"padrao|diferenciado_60|reduzido_zero"}

Redação: legal_base deve ser apenas a referência normativa (curta). justification deve complementar com o raciocínio factual, evitando eco (não copiar o mesmo trecho de legal_base nem repetir "conforme Art. X" se Art. X já está em legal_base).`

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

// strategyUserMessageCompanyContextMaxRunes limita PII/tokens no resumo enviado ao modelo de insight.
const strategyUserMessageCompanyContextMaxRunes = 200

// StrategySOP orienta o modelo a produzir um insight curto, educativo e não normativo
// com base apenas nos números do simulador (sem inventar alíquotas ou parecer fiscal).
const StrategySOP = `Você é um assistente do simulador TribIA (reforma tributária CBS/IBS, cenário ilustrativo).

TAREFA: em até 2 frases curtas em português do Brasil, sugira UMA linha de raciocínio estratégico para o gestor, com base APENAS nos dados numéricos e no regime/perfil informados pelo usuário.

DIRETRIZES:
- Tom educativo; não é parecer fiscal nem consultoria legal. Não cite artigos da lei salvo se o utilizador já os tiver mencionado no contexto da empresa.
- Considere quando fizer sentido: Simples Nacional vs regime regular (apenas como lembrete de que o simulador usa premissas); créditos de insumos e não-cumulatividade em linha geral; saldo credor quando o imposto líquido projetado for negativo (ativo recuperável ilustrativo); perfil exportador (saída desonerada e créditos nas compras, sempre como ilustração do modelo); delta positivo no simulador = aumento de carga líquida projetada vs. atual.
- Não invente valores, percentuais ou normas que não estejam nos dados fornecidos.
- Sem markdown, sem listas, sem aspas no início ou fim. Máximo ~250 caracteres (seja breve).`

// TaxBreakdownSummary agrega valores monetários já formatados como na API HTTP.
type TaxBreakdownSummary struct {
	GrossTax string
	Credits  string
	NetTax   string
}

// BuildStrategyUserMessage monta o pedido compacto para o insight estratégico.
func BuildStrategyUserMessage(regime string, year int, current, projected TaxBreakdownSummary, delta, deltaPct, companyContext string) string {
	var sb strings.Builder
	sb.WriteString("Dados do simulador (ilustrativos):\n")
	sb.WriteString(fmt.Sprintf("Ano de transição: %d\n", year))
	if strings.TrimSpace(regime) != "" {
		sb.WriteString(fmt.Sprintf("Regime/perfil declarado: %s\n", strings.TrimSpace(regime)))
	}
	sb.WriteString("Regime atual — bruto: ")
	sb.WriteString(current.GrossTax)
	sb.WriteString(", créditos: ")
	sb.WriteString(current.Credits)
	sb.WriteString(", líquido: ")
	sb.WriteString(current.NetTax)
	sb.WriteString("\nProjetado — bruto: ")
	sb.WriteString(projected.GrossTax)
	sb.WriteString(", créditos: ")
	sb.WriteString(projected.Credits)
	sb.WriteString(", líquido: ")
	sb.WriteString(projected.NetTax)
	sb.WriteString("\nDelta (projetado − atual): ")
	sb.WriteString(delta)
	sb.WriteString(" (")
	sb.WriteString(deltaPct)
	sb.WriteString("% relativo ao líquido atual, conforme API)\n")

	cc := strings.TrimSpace(companyContext)
	if cc != "" {
		r := []rune(cc)
		if len(r) > strategyUserMessageCompanyContextMaxRunes {
			cc = string(r[:strategyUserMessageCompanyContextMaxRunes]) + "…"
		}
		sb.WriteString("Contexto da empresa (resumo): ")
		sb.WriteString(cc)
		sb.WriteString("\n")
	}
	sb.WriteString("\nGere o insight estratégico conforme as instruções do sistema.")
	return sb.String()
}

// LeakageSOP instrui a LLM a preencher apenas reason e fix para despesas já marcadas inelegíveis.
const LeakageSOP = `Você é um consultor tributário (LC 68/2024, CBS/IBS). O simulador TribIA já calculou value e lost_credit em Go; NÃO os altere.

Tarefa: para cada item, preencha "reason" (por que o crédito não foi apropriado neste modelo, em 1–3 frases) e "fix" (ação prática ilustrativa: documentação, fornecedor, nexo com a atividade, etc.).

Diretrizes:
- Tom conservador; não garanta benefício sem requisitos reais (NF, enquadramento, comprovação de uso na atividade).
- Mencione fornecedor no Simples, uso pessoal, falta de nexo causal ou serviço internacional quando fizer sentido ao caso.
- Não invente artigos; pode citar dispositivos de forma genérica só se estiver seguro (ex.: não-cumulatividade, documento fiscal).

Resposta: APENAS um objeto JSON com a chave "leaks" contendo um array com o MESMO número de elementos e a MESMA ordem do input. Cada elemento deve repetir description, value, lost_credit, regime_type e acrescentar reason e fix. Sem markdown, sem texto fora do JSON.`

// BuildLeakageUserMessage monta o pedido com contexto da empresa e o JSON dos itens.
func BuildLeakageUserMessage(companyRegime, companyContext string, itemsJSON string) string {
	var sb strings.Builder
	sb.WriteString("company_regime: ")
	sb.WriteString(strings.TrimSpace(companyRegime))
	sb.WriteString("\n")
	cc := strings.TrimSpace(companyContext)
	if len(cc) > 2000 {
		cc = cc[:2000] + "…"
	}
	if cc != "" {
		sb.WriteString("contexto_empresa: ")
		sb.WriteString(cc)
		sb.WriteString("\n")
	}
	sb.WriteString("\nItens (ordem obrigatória):\n")
	sb.WriteString(itemsJSON)
	return sb.String()
}
