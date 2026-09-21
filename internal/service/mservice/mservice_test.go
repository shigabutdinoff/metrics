package mservice

import (
	"context"
	"errors"
	"testing"

	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

type fakeRepo struct {
	err      error
	gauges   storage.Gauges
	counters storage.Counters
}

func (f *fakeRepo) BulkUpsert(_ context.Context, gauges storage.Gauges, counters storage.Counters) error {
	f.gauges = gauges
	f.counters = counters
	return f.err
}

func TestUpsert(t *testing.T) {
	ctx := context.Background()
	st := storage.NewMemStorage()
	value := 12.5
	delta := int64(7)
	st.SetGauge(ctx, "Alloc", metrics.GaugeValue(&value))
	st.AddCounter(ctx, "PollCount", metrics.CounterValue(&delta))

	repo := &fakeRepo{}
	if err := Upsert(ctx, repo, st); err != nil {
		t.Fatalf("Upsert() ошибка = %v", err)
	}

	if got := repo.gauges["Alloc"]; len(repo.gauges) != 1 || got == nil || *got != 12.5 {
		t.Fatalf("gauges = %v, ожидается Alloc=12.5", repo.gauges)
	}
	if got := repo.counters["PollCount"]; len(repo.counters) != 1 || got == nil || *got != 7 {
		t.Fatalf("counters = %v, ожидается PollCount=7", repo.counters)
	}
}

func TestUpsert_RepoError(t *testing.T) {
	errRepo := errors.New("репозиторий недоступен")

	err := Upsert(context.Background(), &fakeRepo{err: errRepo}, storage.NewMemStorage())

	if !errors.Is(err, errRepo) {
		t.Fatalf("Upsert() ошибка = %v, ожидается %v", err, errRepo)
	}
}
