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

// BearerToken expõe o JWT cru do header Authorization (sem prefixo Bearer), ou vazio.
func BearerToken(r *http.Request) string {
	return bearerToken(r)
}

// OptionalUserClaims valida o JWT quando presente; devolve (sub, claims, ok).
// ok é false quando não há Bearer. Erro quando há Bearer mas o token é inválido.
func (v *ClerkVerifier) OptionalUserClaims(r *http.Request) (sub string, claims jwt.MapClaims, ok bool, err error) {
	raw := bearerToken(r)
	if raw == "" {
		return "", nil, false, nil
	}
	token, err := jwt.Parse(raw, v.kf.Keyfunc)
	if err != nil {
		return "", nil, true, fmt.Errorf("jwt: %w", err)
	}
	if !token.Valid {
		return "", nil, true, errors.New("jwt invalido")
	}
	c, cok := token.Claims.(jwt.MapClaims)
	if !cok {
		return "", nil, true, errors.New("claims invalidos")
	}
	s, _ := c["sub"].(string)
	if strings.TrimSpace(s) == "" {
		return "", nil, true, errors.New("sub ausente")
	}
	return strings.TrimSpace(s), c, true, nil
}
