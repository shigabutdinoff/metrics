package grpcserver

import (
	"math"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/shigabutdinoff/metrics/internal/audit"
	pb "github.com/shigabutdinoff/metrics/internal/proto"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

type fakeNotifier struct {
	events []audit.Event
}

func (f *fakeNotifier) Publish(e audit.Event) {
	f.events = append(f.events, e)
}

func request(metrics ...*pb.Metric) *pb.UpdateMetricsRequest {
	return pb.UpdateMetricsRequest_builder{Metrics: metrics}.Build()
}

func gauge(id string, v float64) *pb.Metric {
	return pb.Metric_builder{Id: id, Type: pb.Metric_GAUGE, Value: v}.Build()
}

func counter(id string, d int64) *pb.Metric {
	return pb.Metric_builder{Id: id, Type: pb.Metric_COUNTER, Delta: d}.Build()
}

func TestMetricsServer_UpdateMetrics(t *testing.T) {
	st := storage.NewMemStorage()
	n := &fakeNotifier{}
	changes := 0
	s := &MetricsServer{Storage: st, Auditor: n, OnChange: func() { changes++ }}
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 34567}})

	_, err := s.UpdateMetrics(ctx, request(gauge("Alloc", 12.5), counter("PollCount", 5), counter("PollCount", 2)))
	require.NoError(t, err)

	require.Equal(t, 12.5, *st.GetGauge(ctx, "Alloc"))
	require.Equal(t, int64(7), *st.GetCounter(ctx, "PollCount"))
	require.Equal(t, 1, changes)
	require.Len(t, n.events, 1)
	require.Equal(t, []string{"Alloc", "PollCount", "PollCount"}, n.events[0].Metrics)
	require.Equal(t, "10.0.0.5", n.events[0].IPAddress)
	require.NotZero(t, n.events[0].TS)
}

func TestMetricsServer_UpdateMetrics_Invalid(t *testing.T) {
	tests := []struct {
		name string
		req  *pb.UpdateMetricsRequest
	}{
		{name: "пустая пачка", req: request()},
		{name: "пустое имя", req: request(gauge(" ", 1))},
		{name: "неизвестный тип", req: request(pb.Metric_builder{Id: "X", Type: pb.Metric_MType(7)}.Build())},
		{name: "NaN", req: request(gauge("Alloc", math.NaN()))},
		{name: "+Inf", req: request(gauge("Alloc", math.Inf(1)))},
		{name: "частичная пачка", req: request(gauge("Alloc", 1), gauge(" ", 1))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &fakeNotifier{}
			changes := 0
			st := storage.NewMemStorage()
			s := &MetricsServer{Storage: st, Auditor: n, OnChange: func() { changes++ }}

			_, err := s.UpdateMetrics(t.Context(), tt.req)

			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.Nil(t, st.GetGauge(t.Context(), "Alloc"))
			require.Zero(t, changes)
			require.Empty(t, n.events)
		})
	}
}

func TestMetricsServer_UpdateMetrics_NoHooks(t *testing.T) {
	st := storage.NewMemStorage()
	s := &MetricsServer{Storage: st}

	_, err := s.UpdateMetrics(t.Context(), request(gauge("Alloc", 1)))

	require.NoError(t, err)
	require.Equal(t, 1.0, *st.GetGauge(t.Context(), "Alloc"))
}
