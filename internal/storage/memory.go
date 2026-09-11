package storage

import (
	"context"
	"sync"

	"github.com/shigabutdinoff/metrics/internal/model/metrics"
)

// Storage контракт хранилища метрик.
type Storage interface {
	// SetGauge перезаписывает значение gauge-метрики.
	SetGauge(ctx context.Context, name string, value metrics.GaugeValue)
	// AddCounter прибавляет дельту к counter-метрике.
	AddCounter(ctx context.Context, name string, delta metrics.CounterValue)
	// GetGauges отдаёт копию всех gauge-метрик.
	GetGauges(ctx context.Context) Gauges
	// GetCounters отдаёт копию всех counter-метрик.
	GetCounters(ctx context.Context) Counters
	// GetGauge отдаёт значение gauge-метрики или nil, если её нет.
	GetGauge(ctx context.Context, name string) metrics.GaugeValue
	// GetCounter отдаёт значение counter-метрики или nil, если её нет.
	GetCounter(ctx context.Context, name string) metrics.CounterValue
}

type (
	// Gauges набор gauge-метрик по именам.
	Gauges map[string]metrics.GaugeValue
	// Counters набор counter-метрик по именам.
	Counters map[string]metrics.CounterValue
)

// MemStorage потокобезопасное хранилище метрик в памяти.
type MemStorage struct {
	mu       sync.RWMutex
	gauges   Gauges
	counters Counters
}

// NewMemStorage создаёт пустое хранилище в памяти.
func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(Gauges),
		counters: make(Counters),
	}
}

// SetGauge сохраняет присланный указатель как есть, не копируя значение.
func (ms *MemStorage) SetGauge(_ context.Context, name string, value metrics.GaugeValue) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.gauges == nil {
		ms.gauges = make(Gauges)
	}
	ms.gauges[name] = value
}

// AddCounter меняет уже сохранённое значение по указателю на месте.
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

// GetGauges копирует карту, но не значения: указатели остаются общими.
func (ms *MemStorage) GetGauges(_ context.Context) Gauges {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.gauges == nil {
		return make(Gauges)
	}
	gauges := make(Gauges, len(ms.gauges))
	for k, v := range ms.gauges {
		gauges[k] = v
	}
	return gauges
}

// GetCounters копирует и карту, и значения.
func (ms *MemStorage) GetCounters(_ context.Context) Counters {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.counters == nil {
		return make(Counters)
	}
	counters := make(Counters, len(ms.counters))
	for k, v := range ms.counters {
		if v == nil {
			counters[k] = nil
			continue
		}
		vv := *v
		counters[k] = &vv
	}
	return counters
}

// GetGauge отдаёт исходный указатель, а не копию.
func (ms *MemStorage) GetGauge(_ context.Context, name string) metrics.GaugeValue {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.gauges[name]
}

// GetCounter отдаёт копию: AddCounter меняет значение по указателю на месте
func (ms *MemStorage) GetCounter(_ context.Context, name string) metrics.CounterValue {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	v := ms.counters[name]
	if v == nil {
		return nil
	}
	vv := *v
	return &vv
}
