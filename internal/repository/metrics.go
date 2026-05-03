package repository

import (
	"context"

	"github.com/shigabutdinoff/metrics/internal/storage"
)

type Repository interface {
	BulkUpsert(ctx context.Context, gauges storage.Gauges, counters storage.Counters) error
}
