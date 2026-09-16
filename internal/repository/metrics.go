package repository

import (
	"context"

	"github.com/shigabutdinoff/metrics/internal/storage"
)

// Repository контракт постоянного хранилища метрик.
type Repository interface {
	// BulkUpsert записывает пачку метрик одним запросом.
	BulkUpsert(ctx context.Context, gauges storage.Gauges, counters storage.Counters) error
}
