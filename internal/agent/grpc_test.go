package agent

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	config "github.com/shigabutdinoff/metrics/internal/config/agent"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	pb "github.com/shigabutdinoff/metrics/internal/proto"
)

func TestAgent_SendMetricsGRPC(t *testing.T) {
	tests := []struct {
		name     string
		grpcAddr string
		want     []string
	}{
		{name: "IP известен, x-real-ip передан", want: []string{"127.0.0.1"}},
		{name: "IP неизвестен, x-real-ip нет", grpcAddr: "127.0.0.1:99999", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeMetricsServer{}
			addr := startGRPC(t, f)
			a := &Agent{
				grpcClient: grpcClient(t, addr),
				Config:     config.Config{GRPCAddress: cmp.Or(tt.grpcAddr, addr)},
				Logger:     zap.NewNop(),
			}
			g, d := 1.5, int64(3)
			items := []metrics.Metrics{
				{ID: "cpu", MType: metrics.Gauge, Value: &g},
				{ID: "PollCount", MType: metrics.Counter, Delta: &d},
				{ID: "hist", MType: "histogram"},
			}

			if err := a.sendMetricsGRPC(t.Context(), items); err != nil {
				t.Fatalf("sendMetricsGRPC() ошибка = %v", err)
			}

			if len(f.reqs) != 1 {
				t.Fatalf("запросов = %d, ожидается 1", len(f.reqs))
			}
			got := f.reqs[0].GetMetrics()
			want := []*pb.Metric{
				pb.Metric_builder{Id: "cpu", Type: pb.Metric_GAUGE, Value: 1.5}.Build(),
				pb.Metric_builder{Id: "PollCount", Type: pb.Metric_COUNTER, Delta: 3}.Build(),
			}
			if !slices.EqualFunc(got, want, func(a, b *pb.Metric) bool { return proto.Equal(a, b) }) {
				t.Fatalf("метрики = %v, ожидается %v", got, want)
			}
			if !slices.Equal(f.realIPs[0], tt.want) {
				t.Fatalf("x-real-ip = %q, ожидается %q", f.realIPs[0], tt.want)
			}
		})
	}
}

func TestAgent_SendMetricsGRPC_Status(t *testing.T) {
	tests := []struct {
		name        string
		fake        *fakeMetricsServer
		sendTimeout time.Duration
		want        codes.Code
		wantReqs    int
	}{
		{
			name:     "ошибка сервера",
			fake:     &fakeMetricsServer{err: status.Error(codes.PermissionDenied, "IP агента вне доверенной подсети")},
			want:     codes.PermissionDenied,
			wantReqs: 1,
		},
		{name: "повтор после Unavailable", fake: &fakeMetricsServer{failures: 1}, want: codes.OK, wantReqs: 2},
		{name: "тайм-аут", fake: &fakeMetricsServer{block: true}, sendTimeout: 50 * time.Millisecond, want: codes.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{grpcClient: grpcClient(t, startGRPC(t, tt.fake)), Logger: zap.NewNop(), sendTimeout: tt.sendTimeout}
			g := 1.5

			err := a.sendMetricsGRPC(t.Context(), []metrics.Metrics{{ID: "cpu", MType: metrics.Gauge, Value: &g}})

			if status.Code(err) != tt.want {
				t.Fatalf("код ошибки = %v, ожидается %v", status.Code(err), tt.want)
			}
			if len(tt.fake.reqs) != tt.wantReqs {
				t.Fatalf("запросов = %d, ожидается %d", len(tt.fake.reqs), tt.wantReqs)
			}
		})
	}
}
