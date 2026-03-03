package metrics

// NOTE: Не усложняем пример, вводя иерархическую вложенность структур.
// Органичиваясь плоской моделью.
// Delta и Value объявлены через указатели,
// что бы отличать значение "0", от не заданного значения
// и соответственно не кодировать в структуру.
type Metrics struct {
	ID    string       `json:"id"`
	MType Type         `json:"type"`
	Delta CounterValue `json:"delta,omitempty"`
	Value GaugeValue   `json:"value,omitempty"`
	Hash  string       `json:"hash,omitempty"`
}
