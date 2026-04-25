package database

import (
	"context"
	"database/sql"

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
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO metrics (id, type, delta, value, hash)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (id)
        DO UPDATE SET type = EXCLUDED.type,
                      delta = EXCLUDED.delta,
                      value = EXCLUDED.value,
                      hash  = EXCLUDED.hash`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for name, val := range gauges {
		if _, err := stmt.ExecContext(ctx, name, "gauge", nil, (*float64)(val), nil); err != nil {
			return err
		}
	}

	for name, delta := range counters {
		if _, err := stmt.ExecContext(ctx, name, "counter", (*int64)(delta), nil, nil); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}
