package agent

import (
	"cmp"
	"context"
	"errors"
	"net"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"

	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	pb "github.com/shigabutdinoff/metrics/internal/proto"
)

const grpcRetryPolicy = `{"methodConfig": [{
	"name": [{"service": "metrics.Metrics"}],
	"retryPolicy": {
		"maxAttempts": 4,
		"initialBackoff": "1s",
		"maxBackoff": "5s",
		"backoffMultiplier": 3,
		"retryableStatusCodes": ["UNAVAILABLE"]
	}
}]}`

func newGRPCConn(addr, certFile string) (*grpc.ClientConn, error) {
	if certFile == "" {
		return nil, errors.New("нужен сертификат TLS, флаг -crypto-key или CRYPTO_KEY")
	}
	creds, err := credentials.NewClientTLSFromFile(certFile, "")
	if err != nil {
		return nil, err
	}
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultServiceConfig(grpcRetryPolicy),
	)
}

func grpcHostPort(addr string) string {
	if u, err := url.Parse(addr); err == nil && resolver.Get(u.Scheme) != nil {
		addr = resolver.Target{URL: *u}.Endpoint()
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(strings.Trim(addr, "[]"), "443")
	}
	return addr
}

func (a *Agent) sendMetricsGRPC(ctx context.Context, items []metrics.Metrics) error {
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(a.sendTimeout, defaultSendTimeout))
	defer cancel()
	if ip := a.realIP(ctx); ip != "" {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-real-ip", ip))
	}
	_, err := a.grpcClient.UpdateMetrics(ctx, pb.UpdateMetricsRequest_builder{Metrics: toProto(items)}.Build())
	return err
}

func toProto(items []metrics.Metrics) []*pb.Metric {
	batch := make([]*pb.Metric, 0, len(items))
	for _, it := range items {
		m := pb.Metric_builder{Id: it.ID}
		switch it.MType {
		case metrics.Gauge:
			m.Type = pb.Metric_GAUGE
			m.Value = *it.Value
		case metrics.Counter:
			m.Type = pb.Metric_COUNTER
			m.Delta = *it.Delta
		default:
			continue
		}
		batch = append(batch, m.Build())
	}
	return batch
}
