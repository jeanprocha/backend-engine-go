package tax

import "testing"

// TestTransitionTable_CobreTodosOsAnos garante que transitionRow nunca cai no
// panic de "ano sem linha" para o intervalo suportado.
func TestTransitionTable_CobreTodosOsAnos(t *testing.T) {
	for year := 2026; year <= 2033; year++ {
		row := transitionRow(year)
		if row.Year != year {
			t.Errorf("transitionRow(%d) devolveu linha do ano %d", year, row.Year)
		}
	}
}

// TestTransitionTable_ClampForaDoIntervalo espelha o clamp de RulesForYear:
// anos fora de 2026-2033 caem na borda mais próxima.
func TestTransitionTable_ClampForaDoIntervalo(t *testing.T) {
	if got := transitionRow(2000).Year; got != 2026 {
		t.Errorf("ano < 2026: got %d want 2026", got)
	}
	if got := transitionRow(2099).Year; got != 2033 {
		t.Errorf("ano > 2033: got %d want 2033", got)
	}
}

// TestTransitionTable_ProveninciaDeclarada garante que nenhuma linha da
// tabela entra sem Kind/Note preenchidos — regra de ouro do W7/B2.2 (mesmo
// espírito do internal/lawcorpus para o corpus legal: nenhuma afirmação sem
// fonte declarada).
func TestTransitionTable_ProveninciaDeclarada(t *testing.T) {
	validKinds := map[string]bool{
		"lei_calendario":     true,
		"estimativa_oficial": true,
		"premissa_tribia":    true,
	}
	for _, row := range transitionTable {
		if !validKinds[row.Basis.Kind] {
			t.Errorf("ano %d: RuleBasis.Kind %q inválido ou ausente", row.Year, row.Basis.Kind)
		}
		if row.Basis.Note == "" {
			t.Errorf("ano %d: RuleBasis.Note ausente", row.Year)
		}
	}
}
