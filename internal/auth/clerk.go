package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// ClerkVerifier valida session JWT do Clerk contra o JWKS publicado.
type ClerkVerifier struct {
	kf keyfunc.Keyfunc
}

// NewClerkVerifier cria um verificador a partir da URL do JWKS (Dashboard Clerk → API Keys → JWT).
func NewClerkVerifier(jwksURL string) (*ClerkVerifier, error) {
	u := strings.TrimSpace(jwksURL)
	if u == "" {
		return nil, errors.New("CLERK_JWKS_URL vazia")
	}
	kf, err := keyfunc.NewDefault([]string{u})
	if err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	return &ClerkVerifier{kf: kf}, nil
}

// UserIDFromBearer extrai e valida o JWT do header Authorization e devolve o claim sub.
func (v *ClerkVerifier) UserIDFromBearer(r *http.Request) (string, error) {
	raw := bearerToken(r)
	if raw == "" {
		return "", errors.New("token ausente")
	}
	token, err := jwt.Parse(raw, v.kf.Keyfunc)
	if err != nil {
		return "", fmt.Errorf("jwt: %w", err)
	}
	if !token.Valid {
		return "", errors.New("jwt invalido")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("claims invalidos")
	}
	sub, _ := claims["sub"].(string)
	if strings.TrimSpace(sub) == "" {
		return "", errors.New("sub ausente")
	}
	return sub, nil
}

func bearerToken(r *http.Request) string {
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	const p = "Bearer "
	if !strings.HasPrefix(authz, p) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(authz, p))
}
