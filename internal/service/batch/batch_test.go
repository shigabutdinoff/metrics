package batch

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

type fakeNotifier struct {
	events []audit.Event
}

func (f *fakeNotifier) Publish(e audit.Event) {
	f.events = append(f.events, e)
}

func gauge(id string, v float64) metrics.Metrics {
	return metrics.Metrics{ID: id, MType: metrics.Gauge, Value: &v}
}

func counter(id string, d int64) metrics.Metrics {
	return metrics.Metrics{ID: id, MType: metrics.Counter, Delta: &d}
}

func TestUpdate(t *testing.T) {
	st := storage.NewMemStorage()
	n := &fakeNotifier{}

	items := []metrics.Metrics{
		gauge("Alloc", 12.5),
		counter("PollCount", 5),
		counter("PollCount", 2),
	}

	err := Update(t.Context(), st, n, "10.0.0.5", items)

	require.NoError(t, err)
	require.Equal(t, 12.5, *st.GetGauge(t.Context(), "Alloc"))
	require.Equal(t, int64(7), *st.GetCounter(t.Context(), "PollCount"))
	require.Len(t, n.events, 1)
	require.Equal(t, []string{"Alloc", "PollCount", "PollCount"}, n.events[0].Metrics)
	require.Equal(t, "10.0.0.5", n.events[0].IPAddress)
	require.NotZero(t, n.events[0].TS)
}

func TestUpdate_NilNotifier(t *testing.T) {
	st := storage.NewMemStorage()

	require.NoError(t, Update(t.Context(), st, nil, "", []metrics.Metrics{gauge("Alloc", 1)}))
	require.Equal(t, 1.0, *st.GetGauge(t.Context(), "Alloc"))
}

func TestUpdate_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		items []metrics.Metrics
		want  error
	}{
		{name: "пустая пачка", want: ErrEmpty},
		{name: "пустое имя", items: []metrics.Metrics{gauge(" ", 1)}, want: ErrName},
		{name: "неизвестный тип", items: []metrics.Metrics{{ID: "X", MType: "histogram"}}, want: ErrType},
		{name: "NaN", items: []metrics.Metrics{gauge("Alloc", math.NaN())}, want: ErrValue},
		{name: "+Inf", items: []metrics.Metrics{gauge("Alloc", math.Inf(1))}, want: ErrValue},
		{name: "gauge без value", items: []metrics.Metrics{{ID: "Alloc", MType: metrics.Gauge}}, want: ErrValue},
		{name: "counter без delta", items: []metrics.Metrics{{ID: "Alloc", MType: metrics.Counter}}, want: ErrValue},
		{name: "частичная пачка", items: []metrics.Metrics{gauge("Alloc", 1), gauge(" ", 1)}, want: ErrName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := storage.NewMemStorage()
			n := &fakeNotifier{}

			err := Update(t.Context(), st, n, "", tt.items)

			require.ErrorIs(t, err, tt.want)
			require.Nil(t, st.GetGauge(t.Context(), "Alloc"))
			require.Nil(t, st.GetCounter(t.Context(), "Alloc"))
			require.Empty(t, n.events)
		})
	}
}
