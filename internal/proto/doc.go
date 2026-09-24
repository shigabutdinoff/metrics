// Package proto содержит код, сгенерированный protoc из api/metrics.proto.
package proto

//go:generate protoc -I ../../api --go_out=../.. --go_opt=module=github.com/shigabutdinoff/metrics --go-grpc_out=../.. --go-grpc_opt=module=github.com/shigabutdinoff/metrics --go_opt=default_api_level=API_OPAQUE ../../api/metrics.proto
