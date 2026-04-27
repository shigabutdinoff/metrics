package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/shigabutdinoff/metrics/internal/repository"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

var _ repository.Repository = (*PGRepository)(nil)

type PGRepository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *PGRepository {
	return &PGRepository{db: db}
}

func (r *PGRepository) BulkUpsert(ctx context.Context, gauges storage.Gauges, counters storage.Counters) error {
	total := len(gauges) + len(counters)
	if total == 0 {
		return nil
	}

	const cols = 5
	rows := make([]string, 0, total)
	args := make([]any, 0, total*cols)

	i := 1
	for name, val := range gauges {
		rows = append(rows, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", i, i+1, i+2, i+3, i+4))
		args = append(args, name, "gauge", nil, (*float64)(val), nil)
		i += cols
	}
	for name, delta := range counters {
		rows = append(rows, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", i, i+1, i+2, i+3, i+4))
		args = append(args, name, "counter", (*int64)(delta), nil, nil)
		i += cols
	}

	query := `INSERT INTO metrics (id, type, delta, value, hash) VALUES ` +
		strings.Join(rows, ", ") +
		` ON CONFLICT (id) DO UPDATE SET type  = EXCLUDED.type,
		                                 delta = EXCLUDED.delta,
		                                 value = EXCLUDED.value,
		                                 hash  = EXCLUDED.hash`

	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return nil
}
