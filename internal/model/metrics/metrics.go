package metrics

// Metrics метрика в том виде, в котором она передаётся по JSON-эндпоинтам.
//
// generate:reset
type Metrics struct {
	// ID имя метрики.
	ID string `json:"id"`
	// MType тип метрики: Counter или Gauge.
	MType Type `json:"type"`
	// Delta значение метрики, если MType равен Counter.
	Delta CounterValue `json:"delta,omitempty"`
	// Value значение метрики, если MType равен Gauge.
	Value GaugeValue `json:"value,omitempty"`
	// Hash подпись метрики, в HTTP-слое не используется.
	Hash string `json:"hash,omitempty"`
}
