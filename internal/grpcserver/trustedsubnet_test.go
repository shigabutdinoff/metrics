package grpcserver

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestTrustedSubnet(t *testing.T) {
	_, subnet, err := net.ParseCIDR("192.168.0.0/24")
	require.NoError(t, err)

	const outside = "IP агента вне доверенной подсети"
	const unknown = "IP агента не определён"

	tests := []struct {
		name      string
		subnet    *net.IPNet
		md        metadata.MD
		want      codes.Code
		wantLog   string
		wantLogIP string
	}{
		{name: "IP в подсети проходит", subnet: subnet, md: metadata.Pairs("x-real-ip", "192.168.0.69"), want: codes.OK},
		{name: "IP вне подсети", subnet: subnet, md: metadata.Pairs("x-real-ip", "10.0.0.1"), want: codes.PermissionDenied, wantLog: outside, wantLogIP: "10.0.0.1"},
		{name: "нет метаданных", subnet: subnet, want: codes.PermissionDenied, wantLog: unknown},
		{name: "нет ключа x-real-ip", subnet: subnet, md: metadata.Pairs("token", "1"), want: codes.PermissionDenied, wantLog: unknown},
		{name: "не IP", subnet: subnet, md: metadata.Pairs("x-real-ip", "localhost"), want: codes.PermissionDenied, wantLog: unknown},
		{name: "IPv6 при IPv4 подсети", subnet: subnet, md: metadata.Pairs("x-real-ip", "fe80::1"), want: codes.PermissionDenied, wantLog: outside, wantLogIP: "fe80::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.WarnLevel)
			called := false
			handler := func(context.Context, any) (any, error) {
				called = true
				return &struct{}{}, nil
			}
			ctx := t.Context()
			if tt.md != nil {
				ctx = metadata.NewIncomingContext(ctx, tt.md)
			}
			info := &grpc.UnaryServerInfo{FullMethod: "/metrics.Metrics/UpdateMetrics"}

			_, err := TrustedSubnet(tt.subnet, zap.New(core))(ctx, nil, info, handler)

			require.Equal(t, tt.want, status.Code(err))
			require.Equal(t, tt.want == codes.OK, called)
			if tt.wantLog == "" {
				require.Zero(t, logs.Len())
				return
			}
			entries := logs.FilterMessage(tt.wantLog).All()
			require.Len(t, entries, 1)
			if tt.wantLogIP != "" {
				require.Equal(t, tt.wantLogIP, entries[0].ContextMap()["ip"])
			}
		})
	}
}
