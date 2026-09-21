package metrics

import "testing"

func TestMetrics_Reset(t *testing.T) {
	delta := int64(7)
	value := 12.5
	m := &Metrics{ID: "alloc", MType: Gauge, Delta: &delta, Value: &value, Hash: "abc"}

	m.Reset()

	if m.ID != "" || m.MType != "" || m.Hash != "" {
		t.Fatalf("строковые поля не сброшены: %+v", m)
	}
	// Указатели сохраняются, обнуляется значение под ними.
	if m.Delta == nil || *m.Delta != 0 || delta != 0 {
		t.Fatalf("Delta = %v, ожидается указатель на 0", m.Delta)
	}
	if m.Value == nil || *m.Value != 0 || value != 0 {
		t.Fatalf("Value = %v, ожидается указатель на 0", m.Value)
	}
}

func TestMetrics_Reset_NilValues(t *testing.T) {
	m := &Metrics{ID: "requests", MType: Counter}

	m.Reset()

	if m.ID != "" || m.MType != "" {
		t.Fatalf("строковые поля не сброшены: %+v", m)
	}
	if m.Delta != nil || m.Value != nil {
		t.Fatalf("nil-значения должны остаться nil: %+v", m)
	}
}

func TestMetrics_Reset_NilReceiver(t *testing.T) {
	var m *Metrics

	m.Reset()
}
