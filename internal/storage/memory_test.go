package storage

import (
	"context"
	"testing"

	"github.com/shigabutdinoff/metrics/internal/model/metrics"
)

func TestMemStorage_AddCounter(t *testing.T) {
	ms := &MemStorage{}

	first := int64(2)
	second := int64(5)

	ctx := context.Background()
	ms.AddCounter(ctx, "requests", &first)
	ms.AddCounter(ctx, "requests", &second)

	got := ms.GetCounters(ctx)["requests"]
	if got == nil {
		t.Fatal("counter requests is nil")
	}
	if *got != 7 {
		t.Fatalf("counter requests = %d, want %d", *got, 7)
	}
}

func TestMemStorage_GetCounters(t *testing.T) {
	ms := &MemStorage{}
	got := ms.GetCounters(context.Background())

	if got == nil {
		t.Fatal("GetCounters() returned nil map")
	}
	if len(got) != 0 {
		t.Fatalf("len(GetCounters()) = %d, want 0", len(got))
	}
}

func TestMemStorage_GetGauges(t *testing.T) {
	ms := &MemStorage{}
	got := ms.GetGauges(context.Background())

	if got == nil {
		t.Fatal("GetGauges() returned nil map")
	}
	if len(got) != 0 {
		t.Fatalf("len(GetGauges()) = %d, want 0", len(got))
	}
}

func TestMemStorage_SetGauge(t *testing.T) {
	ms := &MemStorage{}

	v1 := 1.5
	v2 := 2.5
	ctx := context.Background()
	ms.SetGauge(ctx, "alloc", metrics.GaugeValue(&v1))
	ms.SetGauge(ctx, "alloc", metrics.GaugeValue(&v2))

	got := ms.GetGauges(ctx)["alloc"]
	if got == nil {
		t.Fatal("gauge alloc is nil")
	}
	if *got != 2.5 {
		t.Fatalf("gauge alloc = %f, want %f", *got, 2.5)
	}
}

func TestNewMemStorage(t *testing.T) {
	got := NewMemStorage()
	if got == nil {
		t.Fatal("NewMemStorage() returned nil")
	}
	if got.gauges == nil {
		t.Fatal("gauges map is nil")
	}
	if got.counters == nil {
		t.Fatal("counters map is nil")
	}
	if len(got.gauges) != 0 || len(got.counters) != 0 {
		t.Fatalf("unexpected non-empty maps: gauges=%d counters=%d", len(got.gauges), len(got.counters))
	}
}
