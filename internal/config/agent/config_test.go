package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shigabutdinoff/metrics/pkg/jsonconfig"
)

func TestConfig_File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	data := `{
		"address": "localhost:8081",
		"report_interval": "1s",
		"poll_interval": "3s",
		"crypto_key": "/path/to/key.pem",
		"key": "secret",
		"rate_limit": 4,
		"grpc_address": "localhost:3201"
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg Config
	if err := jsonconfig.Load(path, &cfg); err != nil {
		t.Fatalf("Load() ошибка = %v", err)
	}

	want := Config{
		PollIntervalInt64:   3,
		ReportIntervalInt64: 1,
		Address:             "localhost:8081",
		Key:                 "secret",
		CryptoKey:           "/path/to/key.pem",
		RateLimitInt64:      4,
		GRPCAddress:         "localhost:3201",
	}
	if cfg != want {
		t.Fatalf("Load() = %+v, ожидается %+v", cfg, want)
	}
}
