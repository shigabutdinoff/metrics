package metrics

// NOTE: не усложняем пример, вводя иерархическую вложенность структур,
// ограничиваясь плоской моделью.
// Delta и Value объявлены через указатели,
// чтобы отличать значение "0" от незаданного значения
// и соответственно не кодировать его в структуру.
type Metrics struct {
	ID    string       `json:"id"`
	MType Type         `json:"type"`
	Delta CounterValue `json:"delta,omitempty"`
	Value GaugeValue   `json:"value,omitempty"`
	Hash  string       `json:"hash,omitempty"`
}
