package grpcserver

import (
	"context"
	"net"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const realIPKey = "x-real-ip"

// TrustedSubnet пропускает вызов, если IP из метаданных x-real-ip входит в
// subnet, иначе пишет причину в logger и возвращает codes.PermissionDenied.
// subnet не может быть nil.
func TrustedSubnet(subnet *net.IPNet, logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		var ipStr string
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if values := md.Get(realIPKey); len(values) > 0 {
				ipStr = values[0]
			}
		}

		ip := net.ParseIP(ipStr)
		if ip == nil {
			logger.Warn("IP агента не определён",
				zap.String("method", info.FullMethod), zap.String(realIPKey, ipStr))
			return nil, status.Error(codes.PermissionDenied, "IP агента не определён")
		}
		if !subnet.Contains(ip) {
			logger.Warn("IP агента вне доверенной подсети",
				zap.String("method", info.FullMethod), zap.Stringer("ip", ip))
			return nil, status.Error(codes.PermissionDenied, "IP агента вне доверенной подсети")
		}
		return handler(ctx, req)
	}
}
