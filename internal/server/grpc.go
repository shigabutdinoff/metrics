package server

import (
	"time"

	"google.golang.org/grpc"

	"github.com/shigabutdinoff/metrics/internal/grpcserver"
	pb "github.com/shigabutdinoff/metrics/internal/proto"
)

func (s *Server) grpcServer() *grpc.Server {
	ms := &grpcserver.MetricsServer{Storage: s.Storage, OnChange: s.onChange}
	if s.auditor != nil {
		ms.Auditor = s.auditor
	}

	gs := grpc.NewServer(grpc.UnaryInterceptor(grpcserver.TrustedSubnet(s.trustedNet, s.Logger)))
	pb.RegisterMetricsServer(gs, ms)
	return gs
}

func (s *Server) stopGRPC(gs *grpc.Server) {
	t := time.AfterFunc(s.shutdownTimeout, func() {
		s.Logger.Warn("Не удалось штатно остановить gRPC")
		gs.Stop()
	})
	defer t.Stop()
	gs.GracefulStop()
}
