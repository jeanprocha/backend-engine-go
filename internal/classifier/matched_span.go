package classifier

// MatchedSpan delimita um intervalo no CONTEXTO DA EMPRESA enviado ao classificador.
// Start e End são índices baseados em pontos de código Unicode (runas), com início
// inclusivo e fim exclusivo, no mesmo string que a API recebe em context.
type MatchedSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// NormalizeMatchedSpan valida e limita o intervalo ao comprimento do contexto em runas.
// Retorna nil se o contexto for vazio ou se os índices forem inválidos.
func NormalizeMatchedSpan(companyContext string, start, end int) *MatchedSpan {
	runes := []rune(companyContext)
	n := len(runes)
	if n == 0 || start < 0 || end <= start || start >= n {
		return nil
	}
	if end > n {
		end = n
	}
	return &MatchedSpan{Start: start, End: end}
}
