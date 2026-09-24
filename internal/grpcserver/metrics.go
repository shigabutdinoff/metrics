package grpcserver

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	pb "github.com/shigabutdinoff/metrics/internal/proto"
	"github.com/shigabutdinoff/metrics/internal/service/batch"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/shigabutdinoff/metrics/pkg/hostaddr"
)

// MetricsServer принимает метрики по gRPC и сохраняет их в Storage.
type MetricsServer struct {
	// UnimplementedMetricsServer даёт заглушки методов будущих версий сервиса.
	pb.UnimplementedMetricsServer
	// Storage хранилище принятых метрик.
	Storage storage.Storage
	// Auditor получает событие аудита после записи, nil отключает аудит.
	Auditor audit.Notifier
	// OnChange вызывается после записи, nil отключает вызов.
	OnChange func()
}

// UpdateMetrics записывает пачку метрик. Ошибка проверки пачки даёт код
// codes.InvalidArgument, и тогда ничего не записывается.
func (s *MetricsServer) UpdateMetrics(ctx context.Context, in *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	items := toModel(in.GetMetrics())
	if err := batch.Update(ctx, s.Storage, s.Auditor, peerIP(ctx), items); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if s.OnChange != nil {
		s.OnChange()
	}
	return &pb.UpdateMetricsResponse{}, nil
}

func toModel(ms []*pb.Metric) []metrics.Metrics {
	items := make([]metrics.Metrics, 0, len(ms))
	for _, m := range ms {
		it := metrics.Metrics{ID: m.GetId()}
		switch m.GetType() {
		case pb.Metric_GAUGE:
			v := m.GetValue()
			it.MType, it.Value = metrics.Gauge, &v
		case pb.Metric_COUNTER:
			d := m.GetDelta()
			it.MType, it.Delta = metrics.Counter, &d
		}
		items = append(items, it)
	}
	return items
}

func peerIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	return hostaddr.Host(p.Addr.String())
}
