package mservice

import (
	"context"
	"database/sql"

	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

func Upsert(ctx context.Context, db *sql.DB, st storage.Storage, logger *zap.Logger) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO metrics (id, type, delta, value, hash)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (id)
        DO UPDATE SET type = EXCLUDED.type,
                      delta = EXCLUDED.delta,
                      value = EXCLUDED.value,
                      hash  = EXCLUDED.hash`)

	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer func() { _ = stmt.Close() }()

	for name, val := range st.GetGauges(ctx) {
		if _, err := stmt.ExecContext(ctx, name, "gauge", nil, (*float64)(val), nil); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	for name, delta := range st.GetCounters(ctx) {
		if _, err := stmt.ExecContext(ctx, name, "counter", (*int64)(delta), nil, nil); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}
