package classifier

import (
	"strings"
	"testing"
)

func TestTruncateStrategyInsightRunes(t *testing.T) {
	if got := truncateStrategyInsightRunes("  abç  ", 10); got != "abç" {
		t.Fatalf("trim: got %q", got)
	}
	long := strings.Repeat("á", 300) // 300 runes
	got := truncateStrategyInsightRunes(long, 250)
	r := []rune(got)
	if len(r) != 250 {
		t.Fatalf("len runes: got %d want 250", len(r))
	}
	if r[len(r)-1] != '…' {
		t.Fatalf("expected ellipsis suffix, got last rune %q", r[len(r)-1])
	}
}

func TestBuildStrategyUserMessage_TruncatesCompanyContext(t *testing.T) {
	longCtx := strings.Repeat("x", 500) // 500 runes
	msg := BuildStrategyUserMessage("regular", 2028,
		TaxBreakdownSummary{"1", "2", "3"},
		TaxBreakdownSummary{"4", "5", "6"},
		"10.00", "5.00",
		longCtx,
		"",
	)
	if !strings.Contains(msg, "Contexto da empresa (resumo):") {
		t.Fatal("missing context label")
	}
	// Após o rótulo, o trecho truncado deve terminar com …
	idx := strings.Index(msg, "Contexto da empresa (resumo): ")
	if idx < 0 {
		t.Fatal("no context prefix")
	}
	rest := msg[idx+len("Contexto da empresa (resumo): "):]
	// até newline
	line := rest
	if p := strings.IndexByte(rest, '\n'); p >= 0 {
		line = rest[:p]
	}
	r := []rune(line)
	if len(r) != strategyUserMessageCompanyContextMaxRunes+1 { // + ellipsis rune
		t.Fatalf("context line runes: got %d want %d+1", len(r), strategyUserMessageCompanyContextMaxRunes)
	}
	if !strings.HasSuffix(line, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", line)
	}
}
