package main

import (
	"strings"
	"testing"
)

func TestClean_CamaraPLPProfile_RemovesHeaderAndFooter(t *testing.T) {
	input := "C Â M A R A  D O S  D E P U T A D O S\n" +
		"PROJETO DE LEI COMPLEMENTAR Nº 68, DE 2024\n" +
		"\n" +
		"Art. 1º Este é\n" +
		"o primeiro artigo.\n" +
		"\n" +
		"Página 3\n" +
		"\n" +
		"Apresentação: 22/07/2024 14:00 - PLP 68/2024\n"

	got := Clean(input, CamaraPLPProfile())

	if strings.Contains(got, "DEPUTADOS") {
		t.Errorf("cabeçalho da Câmara não foi removido: %q", got)
	}
	if strings.Contains(got, "PROJETO DE LEI") {
		t.Errorf("título repetido não foi removido: %q", got)
	}
	if strings.Contains(got, "Página 3") {
		t.Errorf("marcador de página não foi removido: %q", got)
	}
	if strings.Contains(got, "Apresentação") {
		t.Errorf("rodapé de apresentação não foi removido: %q", got)
	}
	if !strings.Contains(got, "#### Art. 1º") {
		t.Errorf("âncora Art. 1º não foi gerada: %q", got)
	}
	if !strings.Contains(got, "Art. 1º Este é o primeiro artigo.") {
		t.Errorf("parágrafo quebrado em duas linhas não foi remontado: %q", got)
	}
}

func TestClean_ParagraphReconstruction_AbbreviationDoesNotClose(t *testing.T) {
	input := "Art. 2º Conforme o art.\n" +
		"308, aplica-se a regra.\n"

	got := Clean(input, NoneProfile())

	if !strings.Contains(got, "Conforme o art. 308, aplica-se a regra.") {
		t.Errorf("abreviação jurídica 'art.' fechou o parágrafo indevidamente: %q", got)
	}
}

func TestClean_PlanaltoDOUProfile_RemovesInstitutionalNoise(t *testing.T) {
	input := "Presidência da República\n" +
		"Casa Civil\n" +
		"Subchefia para Assuntos Jurídicos\n" +
		"\n" +
		"Art. 1º Institui o IBS.\n" +
		"\n" +
		"Este texto não substitui o publicado no DOU\n"

	got := Clean(input, PlanaltoDOUProfile())

	if strings.Contains(got, "Presidência") || strings.Contains(got, "Casa Civil") || strings.Contains(got, "Subchefia") {
		t.Errorf("ruído institucional do Planalto não foi removido: %q", got)
	}
	if strings.Contains(got, "não substitui") {
		t.Errorf("rodapé 'não substitui o publicado no DOU' não foi removido: %q", got)
	}
	if !strings.Contains(got, "#### Art. 1º Institui o IBS.") {
		t.Errorf("âncora do artigo não foi gerada corretamente: %q", got)
	}
}

func TestClean_NoneProfile_OnlySharedRules(t *testing.T) {
	input := "Ruído qualquer que não é removido\n" +
		"\n" +
		"Art. 1º Texto do artigo.\n" +
		"\n" +
		"Página 9\n"

	got := Clean(input, NoneProfile())

	if !strings.Contains(got, "Ruído qualquer") {
		t.Errorf("NoneProfile não deveria remover ruído específico de fonte: %q", got)
	}
	if strings.Contains(got, "Página 9") {
		t.Errorf("marcador de página (regra compartilhada) deveria ser removido mesmo em NoneProfile: %q", got)
	}
}

func TestProfiles_UnknownNameNotPresent(t *testing.T) {
	if _, ok := Profiles()["inexistente"]; ok {
		t.Error("perfil inexistente não deveria estar no catálogo")
	}
	for _, name := range []string{"camara-plp", "planalto-dou", "none"} {
		if _, ok := Profiles()[name]; !ok {
			t.Errorf("perfil %q deveria estar no catálogo", name)
		}
	}
}
