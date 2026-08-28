package ingestion

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Metadados de ancoragem ao PDF oficial (documento default hoje ingerido —
// ver ingestion.DefaultDocumentProfile).
const (
	MetaPdfPage       = "pdf_page"
	MetaPdfCoordY     = "pdf_coord_y"
	MetaLeiPDFVersion = "lei_pdf_version"
	MetaPdfConvention = "pdf_coord_convention"
)

// Convenção fixa no MVP: coordenada Y normalizada 0–1 (topo→fundo da página).
const PdfCoordConventionYNormalized01 = "y_normalized_0_1"

// ExpectedLeiPDFMapVersions são as versões de mapa que LoadLeiArticlePageMap
// aceita. Lista, não valor único: quando a Onda 2 gerar o mapa da LC 214/2025,
// basta ACRESCENTAR a versão nova aqui — o mapa da LC 68/2024 continua
// carregando (dois documentos podem coexistir no corpus por prefixo de
// article_id, ver ingestion.DocumentProfile). Só remova uma versão quando
// tiver certeza de que nenhum artefato commitado a usa mais.
var ExpectedLeiPDFMapVersions = []string{
	"2024-07-22-doc-plp-68",
	// LC 214/2025, texto ATUALIZADO (compilado, já com a LC 227/2026) —
	// docs/legislacao/leicomplementar-214-16-janeiro-2025-796905-normaatualizada-pl.pdf,
	// 298 páginas, gerado pelo CEDI/Câmara em 16/06/2026 (data de criação nos
	// metadados do PDF, que é o único carimbo de versão que ele traz).
	//
	// Escolhido em vez do PDF do DOU de 16/01/2025 porque o corpus limpo
	// (docs/lc214_2025_limpa.md) é o texto compilado: o DOU original não contém
	// os 57 artigos inseridos pela LC 227/2026, e a ancoragem "Ver lei" cairia
	// na página errada para eles. Os dois artefatos têm os MESMOS 580 artigos.
	"2026-06-16-lc214-atualizada",
}

// pdfFilePorLeiVersion liga a versão do mapa ao ARQUIVO PDF que a gerou.
//
// Existe porque a ancoragem "Ver lei" é POR CHUNK: cada chunk carrega seu
// lei_pdf_version, e com dois documentos no corpus (Onda 2/PR 5) o PDF certo
// para um chunk lc68_ não é o mesmo que para um lc214_. Antes disto o handler
// devolvia um PDF global — depois da virada, um dossiê antigo abriria o PDF da
// LC 214 numa página calculada contra o PDF do PLP 68.
var pdfFilePorLeiVersion = map[string]string{
	"2024-07-22-doc-plp-68":       "DOC-PLP-682024-20240722.pdf",
	"2026-06-16-lc214-atualizada": "leicomplementar-214-16-janeiro-2025-796905-normaatualizada-pl.pdf",
}

// PDFFileForLeiVersion devolve o nome do arquivo PDF correspondente à versão do
// mapa, ou "" se a versão for desconhecida (o chamador decide o fallback).
func PDFFileForLeiVersion(v string) string {
	return pdfFilePorLeiVersion[strings.TrimSpace(v)]
}

func isExpectedLeiPDFMapVersion(v string) bool {
	for _, x := range ExpectedLeiPDFMapVersions {
		if x == v {
			return true
		}
	}
	return false
}

// ArticlePageEntry localiza um artigo no PDF oficial.
type ArticlePageEntry struct {
	Page      int    `json:"page"`
	PdfCoordY string `json:"pdf_coord_y"` // 0–1, string para preservar precisão no JSON
	PrfFile   string `json:"prf_file,omitempty"`
}

// LeiArticlePageMap é o artefacto lc68_article_page_map.json (Opção C).
type LeiArticlePageMap struct {
	LeiVersion string                      `json:"lei_version"`
	PrfFile    string                      `json:"prf_file"`
	Convention string                      `json:"convention"`
	Articles   map[string]ArticlePageEntry `json:"articles"`
}

// LoadLeiArticlePageMap lê e valida o ficheiro JSON do mapa página↔artigo.
func LoadLeiArticlePageMap(path string) (*LeiArticlePageMap, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("ingestion: caminho do mapa PDF vazio")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ingestion: ler mapa PDF: %w", err)
	}
	var m LeiArticlePageMap
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("ingestion: JSON do mapa PDF inválido: %w", err)
	}
	if strings.TrimSpace(m.LeiVersion) == "" {
		return nil, errors.New("ingestion: mapa PDF sem lei_version")
	}
	if !isExpectedLeiPDFMapVersion(m.LeiVersion) {
		return nil, fmt.Errorf("ingestion: lei_version do mapa (%q) não está entre as aceitas (%v); regenere o mapa ou acrescente a versão a ExpectedLeiPDFMapVersions",
			m.LeiVersion, ExpectedLeiPDFMapVersions)
	}
	if m.Articles == nil {
		m.Articles = map[string]ArticlePageEntry{}
	}
	if strings.TrimSpace(m.Convention) == "" {
		m.Convention = PdfCoordConventionYNormalized01
	}
	if m.Convention != PdfCoordConventionYNormalized01 {
		return nil, fmt.Errorf("ingestion: convenção desconhecida no mapa: %q", m.Convention)
	}
	return &m, nil
}

// ApplyLeiArticlePageMap mescla pdf_page, pdf_coord_y e lei_pdf_version em cada chunk
// cujo metadata.article_id exista no mapa.
func ApplyLeiArticlePageMap(chunks []ArticleChunk, m *LeiArticlePageMap) {
	if m == nil || len(chunks) == 0 {
		return
	}
	for i := range chunks {
		meta := chunks[i].Metadata
		if meta == nil {
			continue
		}
		aid := strings.TrimSpace(meta["article_id"])
		if aid == "" {
			continue
		}
		ent, ok := m.Articles[aid]
		if !ok {
			continue
		}
		if ent.Page < 1 {
			continue
		}
		meta[MetaPdfPage] = fmt.Sprintf("%d", ent.Page)
		y := strings.TrimSpace(ent.PdfCoordY)
		if y == "" {
			y = "0.1"
		}
		meta[MetaPdfCoordY] = y
		meta[MetaLeiPDFVersion] = m.LeiVersion
		meta[MetaPdfConvention] = PdfCoordConventionYNormalized01
	}
}
