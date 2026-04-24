// Package config agrega leitura de variáveis de ambiente de deploy (Railway, etc.).
package config

import (
	"os"
	"strings"
)

// Port devolve o endereço de escuta ":PORT" (Railway) ou ":8080" em fallback.
func Port() string {
	p := strings.TrimSpace(os.Getenv("PORT"))
	if p == "" {
		p = "8080"
	}
	if strings.HasPrefix(p, ":") {
		return p
	}
	return ":" + p
}

// IsProduction: ENV=production (CORS, logs JSON, etc. no processo de arranque).
func IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production")
}

// CORSAllowedOrigins: lista em bruto (documentação; o matcher está em http/cors).
func CORSAllowedOrigins() string {
	return strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
}
