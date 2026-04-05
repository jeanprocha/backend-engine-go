package classifier

import "testing"

func TestLowSectorAlignmentWarning_TI_medical(t *testing.T) {
	ctx := "Empresa de desenvolvimento de software SaaS B2B"
	desc := "Insumos Médicos (Gazes/Luvas)"
	if !lowSectorAlignmentWarning(ctx, desc) {
		t.Fatal("expected warning for TI context + medical supplies")
	}
}

func TestLowSectorAlignmentWarning_TI_construction(t *testing.T) {
	ctx := "Microempreendedor individual prestador de serviços de TI"
	desc := "Cimento e Tijolos"
	if !lowSectorAlignmentWarning(ctx, desc) {
		t.Fatal("expected warning for TI context + construction materials")
	}
}

func TestLowSectorAlignmentWarning_health_software(t *testing.T) {
	ctx := "Clínica de cardiologia e exames diagnósticos"
	desc := "AWS Cloud Services"
	if !lowSectorAlignmentWarning(ctx, desc) {
		t.Fatal("expected warning for health context + cloud software")
	}
}

func TestLowSectorAlignmentWarning_aligned_noWarning(t *testing.T) {
	if lowSectorAlignmentWarning("Empresa SaaS B2B", "GitHub Copilot") {
		t.Fatal("unexpected warning for aligned pair")
	}
	if lowSectorAlignmentWarning("Clínica médica", "Insumos Médicos (Gazes/Luvas)") {
		t.Fatal("unexpected warning for aligned health pair")
	}
}
