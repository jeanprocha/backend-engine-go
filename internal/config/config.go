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
// string). Muda junto quando a Onda 2 trocar o PDF oficial.
const defaultLawOfficialPDFFile = "DOC-PLP-682024-20240722.pdf"

// LawOfficialPDFURL: URL pública do PDF oficial (QR code no dossiê PDF,
// ancoragem "Ver lei" na UI). LAW_OFFICIAL_PDF_URL é o nome atual (neutro,
// serve qualquer documento); LC68_OFFICIAL_PDF_URL é aceito por um release
// de transição — remover o fallback quando o Railway estiver com o nome
// novo configurado (checklist da Onda 2).
func LawOfficialPDFURL() string {
	if v := strings.TrimSpace(os.Getenv("LAW_OFFICIAL_PDF_URL")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("LC68_OFFICIAL_PDF_URL"))
}

// LawOfficialPDFFile: nome do arquivo do PDF oficial referenciado no dossiê e
// na ancoragem "Ver lei" — default é o documento realmente ingerido hoje.
func LawOfficialPDFFile() string {
	if v := strings.TrimSpace(os.Getenv("LAW_OFFICIAL_PDF_FILE")); v != "" {
		return v
	}
	return defaultLawOfficialPDFFile
}
