package config

import "testing"

// Com dois documentos no corpus (Onda 2/PR 5), a ancoragem "Ver lei" deixou de
// poder ser uma URL só: um dossiê que citou a LC 68 precisa abrir o PDF do
// PLP 68, e um que citou a LC 214 o PDF da LC 214. Estes testes travam as duas
// metades — o modo novo e a preservação do antigo.

const (
	baseBucket = "https://exemplo.supabase.co/storage/v1/object/public/pdf_lei"
	urlLegado  = "https://exemplo.supabase.co/storage/v1/object/public/pdf_lei/DOC-PLP-682024-20240722.pdf"
)

func TestLawOfficialPDFURLFor_ComBaseMontaPorDocumento(t *testing.T) {
	t.Setenv("LAW_OFFICIAL_PDF_BASE_URL", baseBucket)
	t.Setenv("LAW_OFFICIAL_PDF_URL", urlLegado)

	casos := map[string]string{
		"DOC-PLP-682024-20240722.pdf": baseBucket + "/DOC-PLP-682024-20240722.pdf",
		"lc214.pdf":                   baseBucket + "/lc214.pdf",
	}
	for arquivo, want := range casos {
		if got := LawOfficialPDFURLFor(arquivo); got != want {
			t.Errorf("arquivo %q: got %q, want %q", arquivo, got, want)
		}
	}
}

// Sem a base configurada, o comportamento é o de sempre — definir a base é um
// passo deliberado da virada, não um pré-requisito silencioso que quebraria
// produção no deploy.
func TestLawOfficialPDFURLFor_SemBaseCaiNaURLGlobal(t *testing.T) {
	t.Setenv("LAW_OFFICIAL_PDF_BASE_URL", "")
	t.Setenv("LAW_OFFICIAL_PDF_URL", urlLegado)

	if got := LawOfficialPDFURLFor("qualquer-coisa.pdf"); got != urlLegado {
		t.Errorf("got %q, want a URL global %q", got, urlLegado)
	}
}

// Arquivo vazio (versão de mapa desconhecida) também cai na URL global, em vez
// de montar uma URL terminada em barra.
func TestLawOfficialPDFURLFor_ArquivoVazioCaiNaURLGlobal(t *testing.T) {
	t.Setenv("LAW_OFFICIAL_PDF_BASE_URL", baseBucket)
	t.Setenv("LAW_OFFICIAL_PDF_URL", urlLegado)

	if got := LawOfficialPDFURLFor(""); got != urlLegado {
		t.Errorf("got %q, want a URL global %q", got, urlLegado)
	}
}

func TestLawOfficialPDFBaseURL_RemoveBarraFinal(t *testing.T) {
	t.Setenv("LAW_OFFICIAL_PDF_BASE_URL", baseBucket+"/")
	t.Setenv("LAW_OFFICIAL_PDF_URL", "")

	if got := LawOfficialPDFBaseURL(); got != baseBucket {
		t.Errorf("got %q, want %q — barra final duplicaria o separador", got, baseBucket)
	}
	if got := LawOfficialPDFURLFor("x.pdf"); got != baseBucket+"/x.pdf" {
		t.Errorf("got %q — separador duplicado", got)
	}
}
