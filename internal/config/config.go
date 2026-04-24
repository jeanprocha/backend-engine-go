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
