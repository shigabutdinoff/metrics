package storage

import "github.com/Shigabutdinoff/metrics/internal/model/metrics"

type Storage interface {
	SetGauge(name string, value metrics.GaugeValue)
	AddCounter(name string, delta metrics.CounterValue)
	GetGauges() Gauges
	GetCounters() Counters
}

type (
	Gauges   map[string]metrics.GaugeValue
	Counters map[string]metrics.CounterValue
)

type MemStorage struct {
	gauges   Gauges
	counters Counters
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(Gauges),
		counters: make(Counters),
	}
}

func (ms *MemStorage) SetGauge(name string, value metrics.GaugeValue) {
	if ms.gauges == nil {
		ms.gauges = make(Gauges)
	}
	ms.gauges[name] = value
}

func (ms *MemStorage) AddCounter(name string, delta metrics.CounterValue) {
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

func (ms *MemStorage) GetGauges() Gauges {
	if ms.gauges == nil {
		return make(Gauges)
	}
	return ms.gauges
}

func (ms *MemStorage) GetCounters() Counters {
	if ms.counters == nil {
		return make(Counters)
	}
	return ms.counters
}
