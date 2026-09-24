package server

import (
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/shigabutdinoff/metrics/internal/grpcserver"
	pb "github.com/shigabutdinoff/metrics/internal/proto"
)

func (s *Server) grpcServer() (*grpc.Server, error) {
	if s.CryptoCert == "" || s.CryptoKey == "" {
		return nil, errors.New("нужны флаги -crypto-cert и -crypto-key")
	}
	creds, err := credentials.NewServerTLSFromFile(s.CryptoCert, s.CryptoKey)
	if err != nil {
		return nil, err
	}
	ms := &grpcserver.MetricsServer{Storage: s.Storage, Auditor: s.notifier(), OnChange: s.onChange}

	opts := []grpc.ServerOption{grpc.Creds(creds)}
	if s.trustedNet != nil {
		opts = append(opts, grpc.UnaryInterceptor(grpcserver.TrustedSubnet(s.trustedNet, s.Logger)))
	}
	gs := grpc.NewServer(opts...)
	pb.RegisterMetricsServer(gs, ms)
	return gs, nil
}

func (s *Server) stopGRPC(gs *grpc.Server) {
	t := time.AfterFunc(s.shutdownTimeout, func() {
		s.Logger.Warn("Не удалось штатно остановить gRPC")
		gs.Stop()
	})
	defer t.Stop()
	gs.GracefulStop()
}
