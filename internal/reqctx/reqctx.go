// Package reqctx carrega o ID de correlação de uma requisição HTTP através do
// context.Context, para que pacotes de camadas inferiores (ex.:
// internal/classifier) possam correlacionar logs com o request_id sem
// importar internal/transport/http (que já importa esses pacotes — importar
// de volta criaria ciclo). internal/transport/http grava o valor aqui; quem
// só precisa ler (logging) importa só este pacote.
package reqctx

import "context"

type ctxKey struct{}

// WithID grava o request ID no contexto (chamado pelo middleware HTTP).
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext lê o request ID gravado por WithID; string vazia se ausente
// (ex.: contexto de teste ou chamada fora do caminho HTTP).
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}
