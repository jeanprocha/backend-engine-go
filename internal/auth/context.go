package auth

import "context"

type ctxKey int

const userIDKey ctxKey = 1

// ContextWithUserID associa o user id autenticado ao contexto (sub do JWT Clerk).
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext retorna o user id injetado pelo middleware de autenticação.
func UserIDFromContext(ctx context.Context) (string, bool) {
	s, ok := ctx.Value(userIDKey).(string)
	return s, ok && s != ""
}
