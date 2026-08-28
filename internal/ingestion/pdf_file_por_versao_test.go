package ingestion

import "testing"

// PDFFileForLeiVersion é a metade "qual documento" da ancoragem por chunk: o
// handler lê o lei_pdf_version gravado no chunk e precisa saber qual PDF abrir.
// Se um mapa novo entrar em ExpectedLeiPDFMapVersions sem entrada aqui, o
// chunk cai no PDF configurado globalmente — que é o documento ERRADO. Este
// teste é o que impede esse descompasso.
func TestPDFFileForLeiVersion_TodaVersaoAceitaTemArquivo(t *testing.T) {
	for _, v := range ExpectedLeiPDFMapVersions {
		if got := PDFFileForLeiVersion(v); got == "" {
			t.Errorf("versão %q é aceita por LoadLeiArticlePageMap mas não tem PDF associado — "+
				"acrescente em pdfFilePorLeiVersion", v)
		}
	}
}

func TestPDFFileForLeiVersion_CasosConhecidos(t *testing.T) {
	casos := map[string]string{
		"2024-07-22-doc-plp-68":       "DOC-PLP-682024-20240722.pdf",
		"2026-06-16-lc214-atualizada": "leicomplementar-214-16-janeiro-2025-796905-normaatualizada-pl.pdf",
		"  2024-07-22-doc-plp-68  ":   "DOC-PLP-682024-20240722.pdf", // espaços nas pontas
	}
	for v, want := range casos {
		if got := PDFFileForLeiVersion(v); got != want {
			t.Errorf("versão %q: got %q, want %q", v, got, want)
		}
	}
}

// Versão desconhecida devolve vazio — o chamador decide o fallback, em vez de
// receber um arquivo inventado.
func TestPDFFileForLeiVersion_DesconhecidaDevolveVazio(t *testing.T) {
	for _, v := range []string{"", "   ", "2030-01-01-lei-que-nao-existe"} {
		if got := PDFFileForLeiVersion(v); got != "" {
			t.Errorf("versão %q deveria devolver vazio, veio %q", v, got)
		}
	}
}

// Os dois documentos do corpus não podem apontar o mesmo PDF — seria a falha
// que esta mudança existe para corrigir.
func TestPDFFileForLeiVersion_DocumentosDistintosPDFsDistintos(t *testing.T) {
	lc68 := PDFFileForLeiVersion("2024-07-22-doc-plp-68")
	lc214 := PDFFileForLeiVersion("2026-06-16-lc214-atualizada")
	if lc68 == lc214 {
		t.Fatalf("LC 68 e LC 214 apontam o mesmo PDF (%q)", lc68)
	}
}
