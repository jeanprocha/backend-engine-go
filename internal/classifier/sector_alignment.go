package classifier

import "strings"

// lowSectorAlignmentWarning indica se convém injetar aviso na mensagem ao modelo:
// pares contexto↔despesa tipicamente incoerentes (TI vs insumos médicos/obras; saúde vs software puro).
// Heurística lexical, determinística; não substitui a análise jurídica (regras 1–3 do system prompt).
func lowSectorAlignmentWarning(companyContext, expenseDescription string) bool {
	c := strings.ToLower(strings.TrimSpace(companyContext))
	d := strings.ToLower(strings.TrimSpace(expenseDescription))
	if c == "" || d == "" {
		return false
	}

	techCtx := containsAny(c, []string{
		"saas", "software", "desenvolvimento de software",
		"tecnologia da informação", "tecnologia da informacao",
		"consultoria em ti", "serviços de ti", "servicos de ti",
		"empresa de tecnologia", "empresa de desenvolvimento",
		"microempreendedor individual prestador de serviços de ti",
		"microempreendedor individual prestador de servicos de ti",
		"fábrica de software", "fabrica de software",
		"programador", "prestador de serviços de ti", "prestador de servicos de ti",
	}) || strings.Contains(c, " b2b") || strings.Contains(c, "serviços digitais") || strings.Contains(c, "servicos digitais")

	medicalDesc := containsAny(d, []string{
		"gaze", "luva", "médico", "medico", "hospitalar", "cirúrgic", "cirurgic",
		"insumo médico", "insumo medico", "materiais médicos", "materiais medicos",
		"odontológ", "odontolog", "seringa", "estéril", "esteril",
	})

	constructionDesc := containsAny(d, []string{
		"cimento", "tijolo", "tijolos", "argamassa", "brita", "emboço", "emboco",
		"ferro para construção", "ferro para construcao",
	})

	healthCtx := containsAny(c, []string{
		"clínica", "clinica", "hospital", "cardiologia", "saúde", "saude",
		"médic", "medic", "exames diagnósticos", "exames diagnosticos",
		"odontologia", "laboratório", "laboratorio", "paciente",
	})

	swDesc := containsAny(d, []string{
		"aws", "azure", "gcp", "google cloud", "github", "copilot", "gitlab",
		"hosting", "hospedagem", "licença de software", "licenca de software",
		"microsoft 365", "office 365", "slack", "servidor na nuvem",
	})

	if techCtx && (medicalDesc || constructionDesc) {
		return true
	}
	if healthCtx && swDesc {
		return true
	}
	return false
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
