package http

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jeanprocha/backend-engine-go/internal/plg"
)

type ctxKeyPlgTier struct{}

func contextWithPlgTier(ctx context.Context, tier plg.Plan) context.Context {
	return context.WithValue(ctx, ctxKeyPlgTier{}, tier)
}

// PlgTierFromContext devolve o tier injetado por injectPlgTierIntoRequest, se existir.
func PlgTierFromContext(ctx context.Context) (plg.Plan, bool) {
	v, ok := ctx.Value(ctxKeyPlgTier{}).(plg.Plan)
	return v, ok
}

func planFromClaims(c jwt.MapClaims) plg.Plan {
	if c == nil {
		return plg.PlanFree
	}
	if s, ok := c["tribia_plan"].(string); ok {
		return plg.NormalizePlan(s)
	}
	if pm, ok := c["public_metadata"].(map[string]any); ok {
		if s, ok := pm["tribia_plan"].(string); ok {
			return plg.NormalizePlan(s)
		}
	}
	return plg.PlanFree
}

// trustPlanHeader: TRIBIA_TRUST_PLAN_HEADER=true permite que X-Tribia-Plan eleve o tier
// quando o JWT não traz plano (ilustrativo/staging). Inseguro em produção.
func trustPlanHeader() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("TRIBIA_TRUST_PLAN_HEADER")))
	return v == "true" || v == "1"
}

// resolvePlgUserAndTier resolve utilizador (quando identificável) e tier efetivo para quotas.
// Produção (Clerk): JWT é fonte de verdade; o header nunca eleva face ao JWT (apenas
// MostRestrictivePlan para simular tier inferior). Sem Bearer válido, tier = free.
func (s *Server) resolvePlgUserAndTier(r *http.Request) (userID string, tier plg.Plan, err error) {
	headerPlan := plg.NormalizePlan(r.Header.Get("X-Tribia-Plan"))

	if s.authDevSkip {
		uid := strings.TrimSpace(r.Header.Get("X-User-ID"))
		if uid == "" {
			return "", headerPlan, nil
		}
		return uid, headerPlan, nil
	}
	if s.clerkVerifier == nil {
		return "", headerPlan, nil
	}
	sub, claims, ok, err := s.clerkVerifier.OptionalUserClaims(r)
	if err != nil {
		return "", plg.PlanFree, err
	}
	if !ok {
		return "", plg.PlanFree, nil
	}
	jwtTier := planFromClaims(claims)
	if trustPlanHeader() && jwtTier == plg.PlanFree {
		return sub, headerPlan, nil
	}
	tier = plg.MostRestrictivePlan(jwtTier, headerPlan)
	return sub, tier, nil
}

// injectPlgTierIntoRequest anexa o tier resolvido ao contexto do pedido.
func (s *Server) injectPlgTierIntoRequest(r *http.Request) (*http.Request, error) {
	_, tier, err := s.resolvePlgUserAndTier(r)
	if err != nil {
		return r, err
	}
	return r.WithContext(contextWithPlgTier(r.Context(), tier)), nil
}
