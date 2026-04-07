package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Repo persiste e lê histórico de simulações nas tabelas public.simulations e
// public.simulation_items (Supabase/Postgres).
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo cria o repositório a partir do pool compartilhado com ingestion.Store.
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// TaxBreakdownSnapshot espelha TaxBreakdownResponse para gravação/leitura JSON.
type TaxBreakdownSnapshot struct {
	GrossTax string `json:"gross_tax"`
	Credits  string `json:"credits"`
	NetTax   string `json:"net_tax"`
}

// TransitionSeriesSnapshot espelha TransitionSeriesPoint do DTO HTTP no JSONB.
type TransitionSeriesSnapshot struct {
	Year        int    `json:"year"`
	OldTaxNet   string `json:"old_tax_net"`
	NewTaxNet   string `json:"new_tax_net"`
	TotalTaxNet string `json:"total_tax_net"`
}

// SimulationSnapshot espelha SimulationResponse (valores monetários como string).
type SimulationSnapshot struct {
	Year             int                        `json:"year"`
	CompanyRegime    string                     `json:"company_regime,omitempty"` // ex.: exportadora | aliquota_zero (PDF e reidratação)
	Current          TaxBreakdownSnapshot       `json:"current"`
	Projected        TaxBreakdownSnapshot       `json:"projected"`
	Delta            string                     `json:"delta"`
	DeltaPct         string                     `json:"delta_pct"`
	StrategyInsight  string                     `json:"strategy_insight,omitempty"`
	RevenueTotal     string                     `json:"revenue_total,omitempty"`
	TransitionSeries []TransitionSeriesSnapshot `json:"transition_series,omitempty"`
	CreditLeaks      []CreditLeakSnapshot       `json:"credit_leaks,omitempty"`
}

// CreditLeakSnapshot espelha CreditLeakResponse no JSONB do histórico.
type CreditLeakSnapshot struct {
	Description string `json:"description"`
	Value       string `json:"value"`
	LostCredit  string `json:"lost_credit"`
	Reason      string `json:"reason,omitempty"`
	Fix         string `json:"fix,omitempty"`
	RegimeType  string `json:"regime_type,omitempty"`
}

// ServiceLine linha de serviço a persistir.
type ServiceLine struct {
	Description string
	Amount      string
	ISSRate     string
}

// ExpenseLine linha de despesa a persistir (elegibilidade efetiva pós-IA).
type ExpenseLine struct {
	Description string
	Amount      string
	IsEligible  bool
}

// ClassificationLine resultado da IA por despesa (opcional por descrição).
type ClassificationLine struct {
	Description   string
	IsEligible    bool
	Confidence    float64
	Justification string
	LegalBase     string
	RiskLevel     string
	RegimeType    string
}

// SaveInput agrega o payload de uma simulação concluída no frontend.
type SaveInput struct {
	UserID           string
	OrganizationID   *string
	Year             int
	CompanyContext   string
	Simulation       SimulationSnapshot
	Services         []ServiceLine
	Expenses         []ExpenseLine
	Classifications  []ClassificationLine
}

// Save grava cabeçalho + itens em uma única transação.
func (r *Repo) Save(ctx context.Context, in SaveInput) (uuid.UUID, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return uuid.Nil, errors.New("history: user_id obrigatorio")
	}

	curNet, err := decimal.NewFromString(strings.TrimSpace(in.Simulation.Current.NetTax))
	if err != nil {
		return uuid.Nil, fmt.Errorf("history: current net_tax: %w", err)
	}
	projNet, err := decimal.NewFromString(strings.TrimSpace(in.Simulation.Projected.NetTax))
	if err != nil {
		return uuid.Nil, fmt.Errorf("history: projected net_tax: %w", err)
	}
	delta, err := decimal.NewFromString(strings.TrimSpace(in.Simulation.Delta))
	if err != nil {
		return uuid.Nil, fmt.Errorf("history: delta: %w", err)
	}

	snapJSON, err := json.Marshal(in.Simulation)
	if err != nil {
		return uuid.Nil, fmt.Errorf("history: marshal snapshot: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("history: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const qSim = `
		INSERT INTO public.simulations (
			user_id, organization_id, year, company_context,
			total_current_tax, total_projected_tax, delta_impact,
			simulation_snapshot
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
		RETURNING id`

	var simID uuid.UUID
	err = tx.QueryRow(ctx, qSim,
		in.UserID,
		in.OrganizationID,
		in.Year,
		nullIfEmpty(in.CompanyContext),
		curNet,
		projNet,
		delta,
		snapJSON,
	).Scan(&simID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("history: insert simulation: %w", err)
	}

	const qItem = `
		INSERT INTO public.simulation_items (
			simulation_id, description, amount, is_eligible,
			justification, legal_base, confidence, item_type, regime_type
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	for _, s := range in.Services {
		amt, err := parseAmount(s.Amount)
		if err != nil {
			return uuid.Nil, fmt.Errorf("history: service amount %q: %w", s.Description, err)
		}
		// iss_rate não tem coluna dedicada: armazenamos em legal_base só para serviços.
		iss := strings.TrimSpace(s.ISSRate)
		_, err = tx.Exec(ctx, qItem,
			simID,
			s.Description,
			amt,
			false,
			nil,
			nullIfEmpty(iss),
			nil,
			"service",
			"padrao",
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("history: insert service item: %w", err)
		}
	}

	classByDesc := map[string]ClassificationLine{}
	for _, c := range in.Classifications {
		classByDesc[c.Description] = c
	}

	for _, e := range in.Expenses {
		amt, err := parseAmount(e.Amount)
		if err != nil {
			return uuid.Nil, fmt.Errorf("history: expense amount %q: %w", e.Description, err)
		}
		cl, ok := classByDesc[e.Description]
		var just, legal *string
		var conf *float64
		isElig := e.IsEligible
		regime := "padrao"
		if ok {
			isElig = cl.IsEligible
			if cl.Justification != "" {
				just = &cl.Justification
			}
			if cl.LegalBase != "" {
				legal = &cl.LegalBase
			}
			conf = &cl.Confidence
			if rt := strings.TrimSpace(cl.RegimeType); rt != "" {
				regime = rt
			}
		}
		_, err = tx.Exec(ctx, qItem,
			simID,
			e.Description,
			amt,
			isElig,
			just,
			legal,
			conf,
			"expense",
			regime,
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("history: insert expense item: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("history: commit: %w", err)
	}
	return simID, nil
}

func nullIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func parseAmount(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, errors.New("amount vazio")
	}
	return decimal.NewFromString(s)
}

// Summary é uma linha da listagem por usuário (JSON para o frontend).
// TransitionSeries e StrategyInsight vêm do JSONB simulation_snapshot (sem carregar o registo completo).
type Summary struct {
	ID                uuid.UUID                  `json:"id"`
	CreatedAt         time.Time                  `json:"created_at"`
	Year              int                        `json:"year"`
	CompanyContext    *string                    `json:"company_context,omitempty"`
	DeltaImpact       string                     `json:"delta_impact"`
	TotalProjectedTax string                     `json:"total_projected_tax"`
	TransitionSeries  []TransitionSeriesSnapshot `json:"transition_series,omitempty"`
	StrategyInsight   *string                    `json:"strategy_insight,omitempty"`
}

// ListByUser retorna as simulações mais recentes do usuário.
func (r *Repo) ListByUser(ctx context.Context, userID string, limit int) ([]Summary, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	const q = `
		SELECT id, created_at, year, company_context, delta_impact::text, total_projected_tax::text,
			COALESCE(simulation_snapshot->'transition_series', '[]'::jsonb),
			simulation_snapshot->>'strategy_insight'
		FROM public.simulations
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("history: list: %w", err)
	}
	defer rows.Close()

	var out []Summary
	for rows.Next() {
		var s Summary
		var ctxPtr *string
		var tsRaw []byte
		var insight sql.NullString
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.Year, &ctxPtr, &s.DeltaImpact, &s.TotalProjectedTax, &tsRaw, &insight); err != nil {
			return nil, fmt.Errorf("history: list scan: %w", err)
		}
		s.CompanyContext = ctxPtr
		if len(tsRaw) > 0 && string(tsRaw) != "null" {
			_ = json.Unmarshal(tsRaw, &s.TransitionSeries)
		}
		if insight.Valid {
			t := strings.TrimSpace(insight.String)
			if t != "" {
				s.StrategyInsight = &t
			}
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("history: list rows: %w", err)
	}
	return out, nil
}

// Detail é o payload completo para reidratar o dashboard.
type Detail struct {
	ID             uuid.UUID          `json:"id"`
	CreatedAt      time.Time          `json:"created_at"`
	Year           int                `json:"year"`
	CompanyContext string             `json:"company_context"`
	Simulation     SimulationSnapshot `json:"simulation"`
	Services       []ServiceLine      `json:"services"`
	Expenses       []ExpenseLine      `json:"expenses"`
	Classifications []ClassificationLine `json:"classifications"`
}

// GetByID carrega cabeçalho + itens desde que user_id coincida.
func (r *Repo) GetByID(ctx context.Context, userID string, id uuid.UUID) (*Detail, error) {
	const qHead = `
		SELECT year, company_context,
			total_current_tax::text, total_projected_tax::text, delta_impact::text,
			created_at, simulation_snapshot
		FROM public.simulations
		WHERE id = $1 AND user_id = $2`

	var year int
	var ctxPtr *string
	var curNet, projNet, deltaStr string
	var createdAt time.Time
	var snapRaw []byte

	err := r.pool.QueryRow(ctx, qHead, id, userID).Scan(
		&year, &ctxPtr, &curNet, &projNet, &deltaStr, &createdAt, &snapRaw,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("history: get head: %w", err)
	}

	cc := ""
	if ctxPtr != nil {
		cc = *ctxPtr
	}

	var simSnap SimulationSnapshot
	hasSnap := len(snapRaw) > 0 && string(snapRaw) != "null"
	if hasSnap {
		if err := json.Unmarshal(snapRaw, &simSnap); err != nil {
			return nil, fmt.Errorf("history: unmarshal snapshot: %w", err)
		}
	}

	const qItems = `
		SELECT description, amount::text, COALESCE(is_eligible, false),
			justification, legal_base, confidence, item_type,
			COALESCE(NULLIF(TRIM(regime_type), ''), 'padrao')
		FROM public.simulation_items
		WHERE simulation_id = $1
		ORDER BY item_type DESC, description`

	rows, err := r.pool.Query(ctx, qItems, id)
	if err != nil {
		return nil, fmt.Errorf("history: get items: %w", err)
	}
	defer rows.Close()

	var services []ServiceLine
	var expenses []ExpenseLine
	var classifications []ClassificationLine

	for rows.Next() {
		var desc, amount string
		var isElig bool
		var just, legal *string
		var conf *float64
		var itemType string
		var regimeType string
		if err := rows.Scan(&desc, &amount, &isElig, &just, &legal, &conf, &itemType, &regimeType); err != nil {
			return nil, fmt.Errorf("history: item scan: %w", err)
		}
		switch itemType {
		case "service":
			iss := ""
			if legal != nil {
				iss = *legal
			}
			services = append(services, ServiceLine{
				Description: desc,
				Amount:      amount,
				ISSRate:     iss,
			})
		case "expense":
			expenses = append(expenses, ExpenseLine{
				Description: desc,
				Amount:      amount,
				IsEligible:  isElig,
			})
			cl := ClassificationLine{
				Description: desc,
				IsEligible:  isElig,
				Confidence:  0,
				RiskLevel:   "baixo",
				RegimeType:  regimeType,
			}
			if conf != nil {
				cl.Confidence = *conf
			}
			if just != nil {
				cl.Justification = *just
			}
			if legal != nil {
				cl.LegalBase = *legal
			}
			if strings.TrimSpace(cl.RegimeType) == "" {
				cl.RegimeType = "padrao"
			}
			classifications = append(classifications, cl)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("history: items iter: %w", err)
	}

	// Preferir snapshot JSON completo; fallback se coluna ausente ou vazia (instalações antigas).
	if !hasSnap {
		curDec, _ := decimal.NewFromString(curNet)
		deltaDec, _ := decimal.NewFromString(deltaStr)
		deltaPct := decimal.Zero
		if curDec.IsPositive() {
			deltaPct = deltaDec.Div(curDec).Mul(decimal.NewFromInt(100)).Round(2)
		}
		simSnap = SimulationSnapshot{
			Year: year,
			Current: TaxBreakdownSnapshot{
				NetTax:   curNet,
				GrossTax: "0",
				Credits:  "0",
			},
			Projected: TaxBreakdownSnapshot{
				NetTax:   projNet,
				GrossTax: "0",
				Credits:  "0",
			},
			Delta:    deltaStr,
			DeltaPct: deltaPct.StringFixed(4),
		}
	}

	detail := &Detail{
		ID:              id,
		CreatedAt:       createdAt,
		Year:            year,
		CompanyContext:  cc,
		Services:        services,
		Expenses:        expenses,
		Classifications: classifications,
		Simulation:      simSnap,
	}

	return detail, nil
}
