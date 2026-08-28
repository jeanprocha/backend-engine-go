package tax

import (
	"strings"
	"testing"
)

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

// TestTransitionTableHash_DeterministicoENaoVazio garante o contrato que
// internal/enginevalidation.Build depende: chamadas repetidas devolvem o
// mesmo valor, e o valor nunca é a string vazia (que Build trata como
// "evidência sem carimbo").
func TestTransitionTableHash_DeterministicoENaoVazio(t *testing.T) {
	h1 := TransitionTableHash()
	h2 := TransitionTableHash()
	if h1 == "" {
		t.Fatal("TransitionTableHash() devolveu string vazia")
	}
	if h1 != h2 {
		t.Errorf("TransitionTableHash() não é determinístico: %q != %q", h1, h2)
	}
}

// TestTransitionTableHash_IgnoraNote garante que só os campos numéricos
// entram no hash — editar uma Note (typo, esclarecimento) não pode invalidar
// uma evidência já gravada, já que o selo é sobre os NÚMEROS reproduzidos,
// não sobre a prosa que os documenta.
// TestTransitionTable_CitacoesAuditadasOnda2 trava as citações confirmadas
// contra o texto compilado da LC 214/2025 (docs/lc214_2025_limpa.md, Onda
// 2/W1) — nenhum TODO(W1-onda2) resta em transition_table.go. Se um número de
// artigo mudar aqui sem um teste quebrar, a auditoria regrediu em silêncio.
func TestTransitionTable_CitacoesAuditadasOnda2(t *testing.T) {
	casos := []struct {
		year int
		want string // substring que precisa estar presente na Note
	}{
		{2026, "Art. 346"}, // CBS 0,9%
		{2026, "Art. 343"}, // IBS 0,1%
		{2027, "Art. 542"}, // extinção PIS/COFINS
		{2027, "Art. 347"}, // CBS = referência - 0,1pp
		{2027, "Art. 344"}, // IBS 0,1% fixo
		{2029, "Art. 349"}, // delegação ao Senado/TCU
		{2033, "Art. 349"},
	}
	for _, c := range casos {
		note := transitionRow(c.year).Basis.Note
		if !strings.Contains(note, c.want) {
			t.Errorf("ano %d: Note não cita %q (achado da auditoria Onda 2/W1 regrediu?): %q", c.year, c.want, note)
		}
	}
	for _, row := range transitionTable {
		if strings.Contains(row.Basis.Note, "TODO(W1-onda2)") {
			t.Errorf("ano %d: ainda tem TODO(W1-onda2) pendente na Note — a Onda 2 devia ter resolvido isso: %q", row.Year, row.Basis.Note)
		}
	}
}

func TestTransitionTableHash_IgnoraNote(t *testing.T) {
	before := TransitionTableHash()
	original := transitionTable[0].Basis.Note
	transitionTable[0].Basis.Note = original + " (nota editada só para o teste)"
	after := TransitionTableHash()
	transitionTable[0].Basis.Note = original
	if after != before {
		t.Errorf("hash mudou ao editar só a Note: antes=%q depois=%q", before, after)
	}
}
