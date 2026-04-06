package strategytags

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Row is one persisted strategy tag for UI matching.
type Row struct {
	Pattern     string
	Label       string
	Category    string
	ColorScheme string
}

// Repo reads and inserts strategy_tags.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo builds a Repo from the shared pgx pool.
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// ListAll returns all tags ordered by pattern.
func (r *Repo) ListAll(ctx context.Context) ([]Row, error) {
	const q = `
		SELECT pattern, label, COALESCE(category, ''), color_scheme
		FROM strategy_tags
		ORDER BY pattern ASC
	`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("strategytags list: %w", err)
	}
	defer rows.Close()

	var out []Row
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.Pattern, &row.Label, &row.Category, &row.ColorScheme); err != nil {
			return nil, fmt.Errorf("strategytags scan: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// InsertIgnore inserts one row; returns true if a new row was created.
func (r *Repo) InsertIgnore(ctx context.Context, pattern, label, category, colorScheme string) (inserted bool, err error) {
	p := NormalizePattern(pattern)
	if err := ValidateForInsert(p, label, category); err != nil {
		return false, err
	}
	cs := SanitizeColorScheme(colorScheme)
	cat := strings.TrimSpace(category)

	const q = `
		INSERT INTO strategy_tags (pattern, label, category, color_scheme)
		VALUES ($1, $2, NULLIF($3, ''), $4)
		ON CONFLICT (pattern) DO NOTHING
		RETURNING id
	`
	var id int64
	qerr := r.pool.QueryRow(ctx, q, p, strings.TrimSpace(label), cat, cs).Scan(&id)
	if qerr == pgx.ErrNoRows {
		return false, nil
	}
	if qerr != nil {
		return false, fmt.Errorf("strategytags insert: %w", qerr)
	}
	return true, nil
}
