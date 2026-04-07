package http

import (
	"net/http"
	"strings"

	"github.com/jeanprocha/backend-engine-go/internal/auth"
	"github.com/jeanprocha/backend-engine-go/internal/plg"
)

// plgIdentity resolve utilizador e plano para quotas PLG.
func (s *Server) plgIdentity(r *http.Request) (userID string, tier plg.Plan, err error) {
	return s.resolvePlgUserAndTier(r)
}

func (s *Server) checkPipelineQuota(w http.ResponseWriter, r *http.Request) bool {
	if s.plg == nil {
		return true
	}
	uid, tier, err := s.plgIdentity(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "token invalido")
		return false
	}
	if s.plg.Enforce && strings.TrimSpace(uid) == "" {
		writeError(w, http.StatusUnauthorized, "autenticacao obrigatoria para este ambiente (TRIBIA_PLG_ENFORCE)")
		return false
	}
	allowed, used, limit := s.plg.PipelineAllowed(uid, tier)
	if !allowed {
		writeJSON(w, http.StatusForbidden, ErrorResponse{
			Error: "limite diario de simulacoes atingido no plano Free",
			Code:  "plg_simulation_limit",
			Limit: limit,
			Used:  used,
			Plan:  string(tier),
		})
		return false
	}
	return true
}

func (s *Server) recordSimulationPlg(r *http.Request) {
	if s.plg == nil {
		return
	}
	uid, tier, err := s.plgIdentity(r)
	if err != nil || uid == "" {
		return
	}
	s.plg.RecordSimulation(uid, tier)
}

// GET /plg/quota
func (s *Server) plgQuotaHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "nao autenticado")
		return
	}
	userID = strings.TrimSpace(userID)

	r, err := s.injectPlgTierIntoRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "token invalido")
		return
	}
	tier, _ := PlgTierFromContext(r.Context())

	used := 0
	dailyLimit := 0
	enforcement := s.plg != nil && s.plg.Enforce
	if s.plg != nil {
		used = s.plg.SimulationsToday(userID)
		if tier == plg.PlanFree {
			dailyLimit = s.plg.FreeDailySimulations
		}
	}

	companies, err := s.companies.List(r.Context(), userID)
	if err != nil {
		writeInternalError(w, r, "plg_quota_list_companies", err)
		return
	}
	nCo := len(companies)
	coLimit := 0
	switch tier {
	case plg.PlanFree:
		if s.plg != nil {
			coLimit = s.plg.FreeMaxCompanies
		}
	case plg.PlanPro:
		if s.plg != nil {
			coLimit = s.plg.ProMaxCompanies
		}
	default:
		coLimit = 0
	}

	writeJSON(w, http.StatusOK, PlgQuotaResponse{
		Plan:               string(tier),
		SimulationsToday:   used,
		DailyLimit:         dailyLimit,
		CompaniesCount:     nCo,
		CompanyLimit:       coLimit,
		EnforcementEnabled: enforcement,
	})
}
