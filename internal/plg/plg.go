package plg

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Plan representa o tier TribIA enviado pelo cliente (X-Tribia-Plan ou claim JWT).
type Plan string

const (
	PlanFree    Plan = "free"
	PlanPro     Plan = "pro"
	PlanPremium Plan = "premium"
)

// NormalizePlan devolve free|pro|premium ou free se inválido.
func NormalizePlan(raw string) Plan {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pro":
		return PlanPro
	case "premium":
		return PlanPremium
	default:
		return PlanFree
	}
}

func planPrivilegeRank(p Plan) int {
	switch p {
	case PlanPremium:
		return 2
	case PlanPro:
		return 1
	default:
		return 0
	}
}

// MostRestrictivePlan devolve o plano com menor privilégio (free < pro < premium)
// para quotas: útil para combinar JWT com X-Tribia-Plan sem elevar o tier.
func MostRestrictivePlan(a, b Plan) Plan {
	if planPrivilegeRank(a) <= planPrivilegeRank(b) {
		return a
	}
	return b
}

// Limiter aplica quotas diárias de simulação (Free) e teto de empresas por plano.
// Contadores em memória — adequado a demo/portfolio; produção deve usar Redis/DB.
type Limiter struct {
	mu sync.Mutex
	// userID -> contagem por dia UTC (YYYY-MM-DD)
	sim map[string]struct {
		day string
		n   int
	}

	Enforce bool
	// Max simulações completas por dia (pipeline) no plano Free.
	FreeDailySimulations int
	// Máximo de templates de empresa por utilizador.
	FreeMaxCompanies int
	ProMaxCompanies  int
}

func NewLimiterFromEnv() *Limiter {
	enforce := strings.EqualFold(strings.TrimSpace(os.Getenv("TRIBIA_PLG_ENFORCE")), "true") ||
		strings.TrimSpace(os.Getenv("TRIBIA_PLG_ENFORCE")) == "1"

	freeDaily := 3
	if v := strings.TrimSpace(os.Getenv("TRIBIA_FREE_SIM_DAILY_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			freeDaily = n
		}
	}
	freeCo := 3
	if v := strings.TrimSpace(os.Getenv("TRIBIA_FREE_COMPANY_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			freeCo = n
		}
	}
	proCo := 30
	if v := strings.TrimSpace(os.Getenv("TRIBIA_PRO_COMPANY_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			proCo = n
		}
	}

	return &Limiter{
		sim:                  make(map[string]struct{ day string; n int }),
		Enforce:              enforce,
		FreeDailySimulations: freeDaily,
		FreeMaxCompanies:     freeCo,
		ProMaxCompanies:      proCo,
	}
}

func todayUTC() string {
	return time.Now().UTC().Format("2006-01-02")
}

// PipelineAllowed: true se o utilizador pode iniciar classificação/simulação.
func (l *Limiter) PipelineAllowed(userID string, plan Plan) (allowed bool, used int, limit int) {
	if l == nil || !l.Enforce || userID == "" {
		return true, 0, 0
	}
	if plan != PlanFree {
		return true, 0, 0
	}
	limit = l.FreeDailySimulations
	if limit <= 0 {
		return true, 0, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	day := todayUTC()
	ent := l.sim[userID]
	if ent.day != day {
		ent = struct {
			day string
			n   int
		}{day: day, n: 0}
	}
	used = ent.n
	return used < limit, used, limit
}

// RecordSimulation incrementa o contador após simulação concluída com sucesso.
func (l *Limiter) RecordSimulation(userID string, plan Plan) {
	if l == nil || !l.Enforce || userID == "" || plan != PlanFree {
		return
	}
	if l.FreeDailySimulations <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	day := todayUTC()
	ent := l.sim[userID]
	if ent.day != day {
		ent = struct {
			day string
			n   int
		}{day: day, n: 0}
	}
	ent.n++
	ent.day = day
	l.sim[userID] = ent
}

// SimulationsToday devolve uso actual (para GET /plg/quota).
func (l *Limiter) SimulationsToday(userID string) (used int) {
	if l == nil || userID == "" {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	day := todayUTC()
	ent := l.sim[userID]
	if ent.day != day {
		return 0
	}
	return ent.n
}

// CompanyCreateAllowed verifica teto de empresas antes de criar template.
func (l *Limiter) CompanyCreateAllowed(plan Plan, currentCount int) (allowed bool, max int) {
	if l == nil || !l.Enforce {
		return true, 0
	}
	switch plan {
	case PlanPremium:
		return true, 0
	case PlanPro:
		max = l.ProMaxCompanies
		if max <= 0 {
			return true, 0
		}
		return currentCount < max, max
	default:
		max = l.FreeMaxCompanies
		if max <= 0 {
			return true, 0
		}
		return currentCount < max, max
	}
}
