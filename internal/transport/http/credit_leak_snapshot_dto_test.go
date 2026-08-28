package http

import (
	"encoding/json"
	"testing"

	"github.com/jeanprocha/backend-engine-go/internal/history"
)

// TestCreditLeakSnapshot_RoundTrip prova o "terceiro espelho" (achado da
// PR2, docs/roadmap-execucao.md 4.1) para os campos novos da PR5: valor
// anualizado, esforço, risco, prioridade e base legal precisam sobreviver a
// snapshotCreditLeaksFromDTO → JSON (o que vira JSONB) → creditLeaksDTOFromSnapshot,
// não só existir na resposta ao vivo de POST /simulations.
func TestCreditLeakSnapshot_RoundTrip(t *testing.T) {
	original := []CreditLeakResponse{
		{
			Description: "Licença de software ERP",
			Value:       "3000.00",
			LostCredit:  "30.00",
			Reason:      "Sem nexo documental com a receita tributável.",
			Fix:         "Reclassificar como elegível padrão com a documentação de nexo.",
			RegimeType:  "padrao",
			LegalBase:   "Art. 47, LC 214/2025",
			AnnualValues: []CreditLeakAnnualValueResponse{
				{Year: 2026, LostCredit: "30.00"},
				{Year: 2027, LostCredit: "264.00"},
				{Year: 2033, LostCredit: "795.00"},
			},
			Effort:   "baixo",
			Risk:     "baixo",
			Priority: "baixa",
		},
		{
			// Item sem citação — LegalBase precisa continuar vazio no
			// round-trip, nunca virar algo inventado no caminho.
			Description: "Assinatura sem nexo claro",
			Value:       "500.00",
			LostCredit:  "5.00",
			RegimeType:  "diferenciado_60",
			AnnualValues: []CreditLeakAnnualValueResponse{
				{Year: 2026, LostCredit: "2.00"},
			},
			Effort:   "medio",
			Risk:     "baixo",
			Priority: "baixa",
		},
	}

	snap := snapshotCreditLeaksFromDTO(original)
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var reloaded []history.CreditLeakSnapshot
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	back := creditLeaksDTOFromSnapshot(reloaded)

	if len(back) != len(original) {
		t.Fatalf("len(back) = %d, want %d", len(back), len(original))
	}

	first := back[0]
	if first.LegalBase != "Art. 47, LC 214/2025" {
		t.Errorf("LegalBase = %q, want citação preservada", first.LegalBase)
	}
	if first.Effort != "baixo" || first.Risk != "baixo" || first.Priority != "baixa" {
		t.Errorf("faixas divergiram: got effort=%q risk=%q priority=%q", first.Effort, first.Risk, first.Priority)
	}
	if len(first.AnnualValues) != 3 {
		t.Fatalf("AnnualValues: got %d, want 3", len(first.AnnualValues))
	}
	if first.AnnualValues[0].Year != 2026 || first.AnnualValues[0].LostCredit != "30.00" {
		t.Errorf("AnnualValues[0] = %+v", first.AnnualValues[0])
	}
	if first.AnnualValues[2].Year != 2033 || first.AnnualValues[2].LostCredit != "795.00" {
		t.Errorf("AnnualValues[2] = %+v", first.AnnualValues[2])
	}

	second := back[1]
	if second.LegalBase != "" {
		t.Errorf("LegalBase do item sem citação = %q, want vazio (não inventar)", second.LegalBase)
	}
}

// TestCreditLeaksDTOFromSnapshot_RegistroAntigoSemCamposNovos prova o
// precedente do achado 10: um snapshot gravado antes da PR5 (sem
// annual_values/effort/risk/priority/legal_base no JSONB) precisa
// desserializar sem erro, com os campos novos zerados — nunca fabricados.
func TestCreditLeaksDTOFromSnapshot_RegistroAntigoSemCamposNovos(t *testing.T) {
	oldJSON := `[{"description":"Despesa antiga","value":"1000.00","lost_credit":"10.00","regime_type":"padrao"}]`
	var snap []history.CreditLeakSnapshot
	if err := json.Unmarshal([]byte(oldJSON), &snap); err != nil {
		t.Fatalf("unmarshal registro antigo: %v", err)
	}
	back := creditLeaksDTOFromSnapshot(snap)
	if len(back) != 1 {
		t.Fatalf("len(back) = %d, want 1", len(back))
	}
	if back[0].Effort != "" || back[0].Risk != "" || back[0].Priority != "" || back[0].LegalBase != "" {
		t.Errorf("campos novos deveriam ficar vazios em registro antigo, got %+v", back[0])
	}
	if len(back[0].AnnualValues) != 0 {
		t.Errorf("AnnualValues deveria ficar vazio em registro antigo, got %+v", back[0].AnnualValues)
	}
}
