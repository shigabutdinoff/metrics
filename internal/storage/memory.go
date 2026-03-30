package storage

import (
	"context"
	"sync"

	"github.com/shigabutdinoff/metrics/internal/model/metrics"
)

type Storage interface {
	SetGauge(ctx context.Context, name string, value metrics.GaugeValue)
	AddCounter(ctx context.Context, name string, delta metrics.CounterValue)
	GetGauges(ctx context.Context) Gauges
	GetCounters(ctx context.Context) Counters
}

type (
	Gauges   map[string]metrics.GaugeValue
	Counters map[string]metrics.CounterValue
)

type MemStorage struct {
	mu       sync.RWMutex
	gauges   Gauges
	counters Counters
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(Gauges),
		counters: make(Counters),
	}
}

func (ms *MemStorage) SetGauge(_ context.Context, name string, value metrics.GaugeValue) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.gauges == nil {
		ms.gauges = make(Gauges)
	}
	ms.gauges[name] = value
}

func (ms *MemStorage) AddCounter(_ context.Context, name string, delta metrics.CounterValue) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.counters == nil {
		ms.counters = make(Counters)
	}

	existing := ms.counters[name]
	if existing == nil {
		v := *delta
		ms.counters[name] = &v
		return
	}

	*existing += *delta
}

func (ms *MemStorage) GetGauges(_ context.Context) Gauges {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.gauges == nil {
		return make(Gauges)
	}
	return ms.gauges
}

func (ms *MemStorage) GetCounters(_ context.Context) Counters {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.counters == nil {
		return make(Counters)
	}
	return ms.counters
}
