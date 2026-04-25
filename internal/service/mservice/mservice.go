package mservice

import (
	"context"

	"github.com/shigabutdinoff/metrics/internal/repository"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

func Upsert(ctx context.Context, repo repository.Repository, st storage.Storage) error {
	gauges := st.GetGauges(ctx)
	counters := st.GetCounters(ctx)
	return repo.BulkUpsert(ctx, gauges, counters)
}
