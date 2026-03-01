package storage

type Storage interface {
	SetGauge(name string, value float64)
	AddCounter(name string, delta int64)
}

type MemStorage struct {
	gauges   map[string]float64
	counters map[string]int64
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (ms *MemStorage) SetGauge(name string, value float64) {
	if ms.gauges == nil {
		ms.gauges = make(map[string]float64)
	}
	ms.gauges[name] = value
}

func (ms *MemStorage) AddCounter(name string, delta int64) {
	if ms.counters == nil {
		ms.counters = make(map[string]int64)
	}
	ms.counters[name] += delta
}
