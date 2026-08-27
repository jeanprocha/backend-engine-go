package config

import "testing"

func TestLawOfficialPDFURL_Default(t *testing.T) {
	t.Setenv("LAW_OFFICIAL_PDF_URL", "")
	t.Setenv("LC68_OFFICIAL_PDF_URL", "")
	if got := LawOfficialPDFURL(); got != "" {
		t.Errorf("esperava vazio sem nenhuma env definida, obteve %q", got)
	}
}

func TestLawOfficialPDFURL_PrefersNewName(t *testing.T) {
	t.Setenv("LAW_OFFICIAL_PDF_URL", "https://example.org/novo.pdf")
	t.Setenv("LC68_OFFICIAL_PDF_URL", "https://example.org/antigo.pdf")
	if got, want := LawOfficialPDFURL(), "https://example.org/novo.pdf"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLawOfficialPDFURL_FallsBackToOldName(t *testing.T) {
	t.Setenv("LAW_OFFICIAL_PDF_URL", "")
	t.Setenv("LC68_OFFICIAL_PDF_URL", "https://example.org/antigo.pdf")
	if got, want := LawOfficialPDFURL(), "https://example.org/antigo.pdf"; got != want {
		t.Errorf("got %q, want %q — o fallback para o nome antigo da env deveria funcionar", got, want)
	}
}

func TestLawOfficialPDFFile_Default(t *testing.T) {
	t.Setenv("LAW_OFFICIAL_PDF_FILE", "")
	if got, want := LawOfficialPDFFile(), defaultLawOfficialPDFFile; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLawOfficialPDFFile_Override(t *testing.T) {
	t.Setenv("LAW_OFFICIAL_PDF_FILE", "lc214-2025-oficial.pdf")
	if got, want := LawOfficialPDFFile(), "lc214-2025-oficial.pdf"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
