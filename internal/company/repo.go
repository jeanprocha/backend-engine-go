package company

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Company representa um template de empresa/cliente pré-cadastrado pelo usuário.
// DefaultServices é guardado como JSON bruto para flexibilidade ([]FormServiceDTO).
type Company struct {
	ID              uuid.UUID       `json:"id"`
	UserID          string          `json:"user_id"`
	Name            string          `json:"name"`
	TaxContext      string          `json:"tax_context"`
	DefaultServices json.RawMessage `json:"default_services"`
	CreatedAt       time.Time       `json:"created_at"`
}

// Repo persiste e lê templates de empresas na tabela public.companies.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo cria o repositório a partir do pool compartilhado.
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// List retorna todas as empresas do usuário, ordenadas por nome.
func (r *Repo) List(ctx context.Context, userID string) ([]Company, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("company: user_id obrigatorio")
	}

	const q = `
		SELECT id, user_id, name, COALESCE(tax_context, ''), default_services, created_at
		FROM public.companies
		WHERE user_id = $1
		ORDER BY name ASC`

	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("company: list: %w", err)
	}
	defer rows.Close()

	var out []Company
	for rows.Next() {
		var c Company
		var rawServices []byte
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.TaxContext, &rawServices, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("company: list scan: %w", err)
		}
		if len(rawServices) > 0 {
			c.DefaultServices = json.RawMessage(rawServices)
		} else {
			c.DefaultServices = json.RawMessage("[]")
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("company: list rows: %w", err)
	}
	return out, nil
}

// Create insere uma nova empresa e retorna o UUID gerado.
func (r *Repo) Create(ctx context.Context, c Company) (uuid.UUID, error) {
	if strings.TrimSpace(c.UserID) == "" {
		return uuid.Nil, errors.New("company: user_id obrigatorio")
	}
	if strings.TrimSpace(c.Name) == "" {
		return uuid.Nil, errors.New("company: name obrigatorio")
	}

	services := c.DefaultServices
	if len(services) == 0 {
		services = json.RawMessage("[]")
	}

	const q = `
		INSERT INTO public.companies (user_id, name, tax_context, default_services)
		VALUES ($1, $2, $3, $4::jsonb)
		RETURNING id`

	var id uuid.UUID
	err := r.pool.QueryRow(ctx, q,
		strings.TrimSpace(c.UserID),
		strings.TrimSpace(c.Name),
		strings.TrimSpace(c.TaxContext),
		services,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("company: create: %w", err)
	}
	return id, nil
}

// Delete remove a empresa garantindo que pertence ao userID (sem RLS no driver).
func (r *Repo) Delete(ctx context.Context, userID string, id uuid.UUID) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("company: user_id obrigatorio")
	}

	const q = `DELETE FROM public.companies WHERE id = $1 AND user_id = $2`
	tag, err := r.pool.Exec(ctx, q, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("company: not found")
		}
		return fmt.Errorf("company: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("company: not found or unauthorized")
	}
	return nil
}
