// Package config agrega leitura de variáveis de ambiente de deploy (Railway, etc.).
package config

import (
	"net"
	"os"
	"strings"
)

// ListenAddr devolve o endereço de escuta para http.Server.
// Railway injeta PORT (só o número, ex. 8080). O processo deve ouvir em 0.0.0.0
// (todas as interfaces), nunca em 127.0.0.1/localhost — caso contrário o proxy
// do PaaS recebe 502 Bad Gateway.
func ListenAddr() string {
	p := strings.TrimSpace(os.Getenv("PORT"))
	if p == "" {
		p = "8080"
	}
	p = strings.TrimPrefix(strings.TrimSpace(p), ":")
	// Forma explícita; em Go ":port" também escuta em todas as interfaces, mas
	// 0.0.0.0 deixa o contrato de deploy claro para Railway / health checks.
	return net.JoinHostPort("0.0.0.0", p)
}

// IsProduction: ENV=production (CORS, logs JSON, etc. no processo de arranque).
func IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production")
}

// CORSAllowedOrigins: lista em bruto (documentação; o matcher está em http/cors).
func CORSAllowedOrigins() string {
	return strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
}

// defaultLawOfficialPDFFile é o documento hoje ingerido (PLP 68/2024) — o
// mesmo default de ingestion.DefaultDocumentProfile, mas config não importa
// ingestion (evita acoplar deploy config a schema de ingestão por uma única
// string).
//
// Continua apontando o PLP 68 de propósito: o corpus em produção ainda é o
// lc68_, e trocar este valor antes da ingestão da LC 214 faria o "Ver lei"
// abrir o PDF da LC 214 numa página calculada contra o PDF do PLP 68. Muda na
// PR 6 da Onda 2 (a virada), junto com o fallback de LawOfficialPDFURL.
//
// Papel reduzido desde a Onda 2/PR 6: o /pdf-anchor agora resolve o PDF pelo
// lei_pdf_version DO CHUNK (ingestion.PDFFileForLeiVersion), então este valor
// só é usado quando a versão do chunk é desconhecida — chunk antigo sem mapa,
// ou mapa novo sem entrada correspondente. Continua apontando o PLP 68 de
// propósito: é o documento dos chunks mais antigos, que são justamente os que
// podem chegar sem versão.
const defaultLawOfficialPDFFile = "DOC-PLP-682024-20240722.pdf"

// LawOfficialPDFURL: URL pública do PDF oficial (QR code no dossiê PDF,
// ancoragem "Ver lei" na UI). LAW_OFFICIAL_PDF_URL é o nome atual (neutro,
// serve qualquer documento); LC68_OFFICIAL_PDF_URL é aceito por um release
// de transição.
//
// ⚠️ NÃO remover o fallback ainda. Verificado no Railway em 27/08/2026
// (Onda 2/PR 4): `LC68_OFFICIAL_PDF_URL` está DEFINIDA e `LAW_OFFICIAL_PDF_URL`
// está AUSENTE. Remover hoje faria esta função devolver "" em produção, e daí:
// GET /law/articles/{id}/pdf-anchor passa a responder 503, e a seção "Texto
// oficial" do dossiê em PDF (com o QR) simplesmente some — ambos em silêncio
// para quem não estiver olhando.
//
// Ordem correta: definir LAW_OFFICIAL_PDF_URL no Railway → confirmar que a
// ancoragem funciona → só então remover o fallback. Isso pertence à PR 6 da
// Onda 2 (a virada), junto com defaultLawOfficialPDFFile.
func LawOfficialPDFURL() string {
	if v := strings.TrimSpace(os.Getenv("LAW_OFFICIAL_PDF_URL")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("LC68_OFFICIAL_PDF_URL"))
}

// LawOfficialPDFBaseURL: diretório público onde vivem os PDFs oficiais (o
// bucket do Supabase Storage), SEM barra no fim. Vazio = modo documento único.
//
// Existe porque a ancoragem "Ver lei" passou a ser por documento (Onda 2/PR 6):
// com LC 68 e LC 214 coexistindo no corpus, cada chunk precisa do SEU PDF. Uma
// URL só não dá conta de dois documentos.
func LawOfficialPDFBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("LAW_OFFICIAL_PDF_BASE_URL")), "/")
}

// LawOfficialPDFURLFor resolve a URL do PDF de UM documento, identificado pelo
// nome do arquivo (ver ingestion.PDFFileForLeiVersion).
//
// Com LAW_OFFICIAL_PDF_BASE_URL definida, monta base + arquivo — é o modo que
// serve N documentos. Sem ela, cai na URL global de sempre: o comportamento
// anterior, preservado para que definir a base seja um passo deliberado da
// virada e não um pré-requisito silencioso.
func LawOfficialPDFURLFor(file string) string {
	base := LawOfficialPDFBaseURL()
	file = strings.TrimSpace(file)
	if base != "" && file != "" {
		return base + "/" + file
	}
	return LawOfficialPDFURL()
}

// LawOfficialPDFFile: nome do arquivo do PDF oficial referenciado no dossiê e
// na ancoragem "Ver lei" — default é o documento realmente ingerido hoje.
func LawOfficialPDFFile() string {
	if v := strings.TrimSpace(os.Getenv("LAW_OFFICIAL_PDF_FILE")); v != "" {
		return v
	}
	return defaultLawOfficialPDFFile
}

// RAGDocumentPrefix: delimita a busca semântica a UM documento do corpus, pelo
// prefixo de article_id (ex.: "lc214_"). Vazio (default) = corpus inteiro,
// comportamento de sempre.
//
// W1/Onda 2, PR 1. Enquanto só a LC 68/2024 está ingerida isto é inócuo — os
// 377 chunks têm todos o mesmo prefixo. Passa a ser obrigatório quando a
// LC 214/2025 coexistir no mesmo `tax_law_chunks` (PR 5, coexistência sem
// TRUNCATE): as duas leis são quase-duplicatas semânticas e a busca sem filtro
// devolveria escolhas arbitrárias entre elas.
//
// ⚠️ Só configurar DEPOIS de aplicar docs/migrations/009 no Supabase — antes
// dela a função match_tax_law não aceita o 4º argumento. Sem esta variável, o
// Go emite a chamada de 3 argumentos, que funciona nas duas versões da função.
//
// ⚠️ Acoplamento com o documento corrente: esta variável e o documento que
// GET /law/corpus reporta como corrente (LAW_CORPUS_CURRENT_SOURCE, ou o de
// mais chunks) precisam apontar para o MESMO documento. Divergir significa
// recuperar de uma lei e carimbar o selo com outra — exatamente o que a
// Onda 2 existe para impedir. A PR 6 (virada) muda os dois juntos.
func RAGDocumentPrefix() string {
	return strings.TrimSpace(os.Getenv("RAG_DOCUMENT_PREFIX"))
}
