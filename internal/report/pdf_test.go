package report

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jeanprocha/backend-engine-go/internal/history"
)

func TestGenerateDiagnosticPDF_SmokeAndSignature(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	d := &history.Detail{
		ID:             id,
		CreatedAt:      time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		Year:           2026,
		CompanyContext: "Empresa SaaS ilustrativa",
		Simulation: history.SimulationSnapshot{
			Year: 2026,
			Current: history.TaxBreakdownSnapshot{
				GrossTax: "100.00",
				Credits:  "10.00",
				NetTax:   "90.00",
			},
			Projected: history.TaxBreakdownSnapshot{
				GrossTax: "80.00",
				Credits:  "15.00",
				NetTax:   "65.00",
			},
			Delta:    "-25.00",
			DeltaPct: "-27.78",
		},
		Services: []history.ServiceLine{
			{Description: "Consultoria", Amount: "10000.00", ISSRate: "0.05"},
		},
		Expenses: []history.ExpenseLine{
			{Description: "AWS", Amount: "3000.00", IsEligible: true},
		},
		Classifications: []history.ClassificationLine{
			{
				Description:   "AWS",
				IsEligible:    true,
				Confidence:    0.9,
				Justification: "Insumo de TI",
				LegalBase:     "Art. 28 LC 68/2024 (ilustrativo)",
				RiskLevel:     "baixo",
				RegimeType:    "padrao",
			},
		},
	}

	b, err := GenerateDiagnosticPDF(d)
	if err != nil {
		t.Fatalf("GenerateDiagnosticPDF: %v", err)
	}
	if len(b) < 100 {
		t.Fatalf("PDF muito pequeno: %d bytes", len(b))
	}
	if string(b[:4]) != "%PDF" {
		t.Fatalf("assinatura PDF ausente, prefixo: %q", string(b[:min(16, len(b))]))
	}
}

func TestGenerateDiagnosticPDF_ContentLiterals(t *testing.T) {
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	d := &history.Detail{
		ID:             id,
		CreatedAt:      time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		Year:           2026,
		CompanyContext: "Contexto de teste para lente do documento",
		Simulation: history.SimulationSnapshot{
			Year:          2026,
			CompanyRegime: "regular",
			Current: history.TaxBreakdownSnapshot{
				GrossTax: "100.00",
				Credits:  "10.00",
				NetTax:   "90.00",
			},
			Projected: history.TaxBreakdownSnapshot{
				GrossTax: "80.00",
				Credits:  "15.00",
				NetTax:   "65.00",
			},
			Delta:    "-25.00",
			DeltaPct: "-27.78",
		},
		Services: []history.ServiceLine{
			{Description: "Consultoria", Amount: "10000.00", ISSRate: "0.05"},
		},
		Expenses: []history.ExpenseLine{
			{Description: "AWS", Amount: "3000.00", IsEligible: true},
			{Description: "Spotify", Amount: "30.00", IsEligible: false},
		},
		Classifications: []history.ClassificationLine{
			{
				Description: "AWS", IsEligible: true, Confidence: 0.9,
				Justification: "Insumo", LegalBase: "Art. 28", RiskLevel: "baixo", RegimeType: "padrao",
			},
			{
				Description: "Spotify", IsEligible: false, Confidence: 0.95,
				Justification: "Pessoal", LegalBase: "Art. 28", RiskLevel: "medio", RegimeType: "padrao",
			},
		},
	}

	b, err := GenerateDiagnosticPDF(d)
	if err != nil {
		t.Fatalf("GenerateDiagnosticPDF: %v", err)
	}
	needles := []string{
		"Equacao",
		"Perfil simulador: regular",
		"Elegivel",
		"Inadmissivel",
	}
	for _, n := range needles {
		if !bytes.Contains(b, []byte(n)) {
			t.Fatalf("PDF sem texto literal esperado %q (pode falhar se o gerador passar a comprimir streams)", n)
		}
	}
}

func TestGenerateDiagnosticPDF_ExportadoraSaldoCredorCopy(t *testing.T) {
	id := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	d := &history.Detail{
		ID:        id,
		CreatedAt: time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		Year:      2026,
		Simulation: history.SimulationSnapshot{
			Year:          2026,
			CompanyRegime: "exportadora",
			Current: history.TaxBreakdownSnapshot{
				GrossTax: "100.00", Credits: "10.00", NetTax: "90.00",
			},
			Projected: history.TaxBreakdownSnapshot{
				GrossTax: "0.00", Credits: "500.00", NetTax: "-500.00",
			},
			Delta: "-590.00", DeltaPct: "-100.00",
		},
		Services:        []history.ServiceLine{{Description: "SaaS export", Amount: "50000.00", ISSRate: "0.05"}},
		Expenses:        []history.ExpenseLine{{Description: "AWS", Amount: "2000.00", IsEligible: true}},
		Classifications: []history.ClassificationLine{{Description: "AWS", IsEligible: true, Confidence: 0.9, Justification: "Insumo", LegalBase: "Art. 28", RiskLevel: "baixo", RegimeType: "padrao"}},
	}
	b, err := GenerateDiagnosticPDF(d)
	if err != nil {
		t.Fatalf("GenerateDiagnosticPDF: %v", err)
	}
	for _, n := range []string{"Receita x 0%", "Saldo credor", "Liquido negativo"} {
		if !bytes.Contains(b, []byte(n)) {
			t.Fatalf("PDF exportadora sem texto esperado %q", n)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
