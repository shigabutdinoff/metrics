package agent

import (
	"context"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/shigabutdinoff/metrics/internal/proto"
)

const (
	testCert = "testdata/cert.pem"
	testKey  = "testdata/private.pem"
)

type fakeMetricsServer struct {
	pb.UnimplementedMetricsServer
	mu       sync.Mutex
	reqs     []*pb.UpdateMetricsRequest
	realIPs  [][]string
	err      error
	block    bool
	failures int
}

func (f *fakeMetricsServer) UpdateMetrics(ctx context.Context, in *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	md, _ := metadata.FromIncomingContext(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, in)
	f.realIPs = append(f.realIPs, md.Get("x-real-ip"))
	if len(f.reqs) <= f.failures {
		return nil, status.Error(codes.Unavailable, "сервер недоступен")
	}
	return &pb.UpdateMetricsResponse{}, f.err
}

func startGRPC(t *testing.T, f *fakeMetricsServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	creds, err := credentials.NewServerTLSFromFile(testCert, testKey)
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterMetricsServer(gs, f)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	return lis.Addr().String()
}

func grpcClient(t *testing.T, addr string) pb.MetricsClient {
	t.Helper()
	conn, err := newGRPCConn(addr, testCert)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewMetricsClient(conn)
}
