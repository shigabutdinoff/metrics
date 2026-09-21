package grpcserver

import (
	"context"
	"math"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/shigabutdinoff/metrics/internal/audit"
	pb "github.com/shigabutdinoff/metrics/internal/proto"
	"github.com/shigabutdinoff/metrics/internal/storage"
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

// UpdateMetrics проверяет все метрики пачки и только потом пишет их по одной.
// Пустая пачка, пустое имя, неизвестный тип или gauge NaN и Inf дают ошибку
// с кодом codes.InvalidArgument, и тогда ничего не записывается.
func (s *MetricsServer) UpdateMetrics(ctx context.Context, in *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	ms := in.GetMetrics()
	if err := validate(ms); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(ms))
	for _, m := range ms {
		switch m.GetType() {
		case pb.Metric_GAUGE:
			v := m.GetValue()
			s.Storage.SetGauge(ctx, m.GetId(), &v)
		case pb.Metric_COUNTER:
			d := m.GetDelta()
			s.Storage.AddCounter(ctx, m.GetId(), &d)
		}
		names = append(names, m.GetId())
	}

	if s.OnChange != nil {
		s.OnChange()
	}
	if s.Auditor != nil {
		s.Auditor.Publish(audit.Event{TS: time.Now().Unix(), Metrics: names, IPAddress: peerIP(ctx)})
	}
	return &pb.UpdateMetricsResponse{}, nil
}

func validate(ms []*pb.Metric) error {
	if len(ms) == 0 {
		return status.Error(codes.InvalidArgument, "Отсутствуют метрики")
	}

	for _, m := range ms {
		if strings.TrimSpace(m.GetId()) == "" {
			return status.Error(codes.InvalidArgument, "Неверное название метрики")
		}

		switch m.GetType() {
		case pb.Metric_GAUGE:
			if math.IsNaN(m.GetValue()) || math.IsInf(m.GetValue(), 0) {
				return status.Error(codes.InvalidArgument, "Неверное значение метрики")
			}
		case pb.Metric_COUNTER:
		default:
			return status.Error(codes.InvalidArgument, "Неверный тип метрики")
		}
	}
	return nil
}

func peerIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return p.Addr.String()
	}
	return host
}
