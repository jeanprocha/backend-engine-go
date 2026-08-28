// Package waitlist grava e-mails capturados pelo formulário da landing
// (Etapa M/PR 9) — substitui o CTA morto ("Entrar na lista de espera").
package waitlist

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Join insere um e-mail; joined=false quando o e-mail já estava na lista
// (ON CONFLICT DO NOTHING) — tratado como sucesso pelo handler, não como
// erro: reenviar o mesmo formulário não deve parecer uma falha para quem já
// está inscrito.
func (r *Repo) Join(ctx context.Context, email string) (joined bool, err error) {
	e := NormalizeEmail(email)
	if err := ValidateForInsert(e); err != nil {
		return false, err
	}

	const q = `
		INSERT INTO waitlist (email)
		VALUES ($1)
		ON CONFLICT (email) DO NOTHING
		RETURNING id
	`
	var id int64
	qerr := r.pool.QueryRow(ctx, q, e).Scan(&id)
	if qerr == pgx.ErrNoRows {
		return false, nil
	}
	if qerr != nil {
		return false, fmt.Errorf("waitlist join: %w", qerr)
	}
	return true, nil
}
