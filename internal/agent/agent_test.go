package agent

import (
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	config "github.com/shigabutdinoff/metrics/internal/config/agent"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

func TestAgent_CollectMetrics(t *testing.T) {
	st := storage.NewMemStorage()
	a := &Agent{Storage: st}

	a.CollectMetrics()

	poll := st.GetCounters(context.Background())["PollCount"]
	if poll == nil {
		t.Fatal("счётчик PollCount не установлен")
	}
	if *poll != 1 {
		t.Fatalf("PollCount = %d, ожидается 1", *poll)
	}

	randomValue := st.GetGauges(context.Background())["RandomValue"]
	if randomValue == nil {
		t.Fatal("gauge RandomValue не установлен")
	}

	alloc := st.GetGauges(context.Background())["Alloc"]
	if alloc == nil {
		t.Fatal("Alloc gauge из runtime-статистики не установлен")
	}
}

func TestAgent_SendMetrics_Hash(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		wantHash bool
	}{
		{name: "ключ задан - заголовок HashSHA256 выставлен", key: "secret", wantHash: true},
		{name: "ключ пуст - заголовок HashSHA256 отсутствует", key: "", wantHash: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHeader string
			var hashMatched bool
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// агент шлёт gzip - распаковываем, чтобы проверить хэш несжатого тела
				zr, err := gzip.NewReader(r.Body)
				if err != nil {
					t.Fatalf("gzip.NewReader() ошибка = %v", err)
				}
				defer zr.Close()

				body, err := io.ReadAll(zr)
				if err != nil {
					t.Fatalf("io.ReadAll() ошибка = %v", err)
				}

				gotHeader = r.Header.Get("HashSHA256")
				if tt.key != "" {
					mac := hmac.New(sha256.New, []byte(tt.key))
					mac.Write(body)
					want := hex.EncodeToString(mac.Sum(nil))
					hashMatched = hmac.Equal([]byte(gotHeader), []byte(want))
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer ts.Close()

			g := 1.5
			a := &Agent{
				Storage: storage.NewMemStorage(),
				Client:  resty.NewWithClient(ts.Client()),
				Config:  config.Config{Address: config.Address(ts.URL), Key: tt.key},
			}

			items := []metrics.Metrics{{ID: "cpu", MType: metrics.Gauge, Value: &g}}
			if err := a.sendMetrics(context.Background(), items); err != nil {
				t.Fatalf("sendMetrics() ошибка = %v", err)
			}

			if tt.wantHash {
				if gotHeader == "" {
					t.Fatal("ожидался заголовок HashSHA256, но он отсутствует")
				}
				if !hashMatched {
					t.Fatalf("HashSHA256 не совпал с хэшем несжатого тела: %q", gotHeader)
				}
			} else if gotHeader != "" {
				t.Fatalf("заголовок HashSHA256 не ожидался, получен %q", gotHeader)
			}
		})
	}
}

func TestAgent_Run(t *testing.T) {
	var requests atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a := &Agent{
		Storage:        storage.NewMemStorage(),
		Client:         resty.NewWithClient(ts.Client()),
		Config:         config.Config{Address: config.Address(ts.URL), RateLimitInt64: 2},
		PollInterval:   20 * time.Millisecond,
		ReportInterval: 50 * time.Millisecond,
		Logger:         zap.NewNop(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = a.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился после отмены контекста — возможна утечка горутин или дедлок")
	}

	if requests.Load() == 0 {
		t.Fatal("ни одного запроса с метриками не отправлено")
	}
}

func TestAgent_CollectGopsutilMetrics(t *testing.T) {
	st := storage.NewMemStorage()
	a := &Agent{Storage: st, Logger: zap.NewNop()}

	a.collectGopsutil(context.Background())

	gauges := st.GetGauges(context.Background())

	if gauges["TotalMemory"] == nil {
		t.Fatal("gauge TotalMemory не установлен")
	}
	if gauges["FreeMemory"] == nil {
		t.Fatal("gauge FreeMemory не установлен")
	}

	cpuCount := 0
	for name := range gauges {
		if strings.HasPrefix(name, "CPUutilization") {
			cpuCount++
		}
	}
	if cpuCount != runtime.NumCPU() {
		t.Fatalf("число метрик CPUutilization = %d, ожидается %d (по числу CPU)", cpuCount, runtime.NumCPU())
	}
	if gauges["CPUutilization1"] == nil {
		t.Fatal("gauge CPUutilization1 не установлен")
	}
}

func TestAgent_RateLimit(t *testing.T) {
	const rateLimit = 3

	var inFlight, maxSeen atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := inFlight.Add(1)
		for {
			m := maxSeen.Load()
			if cur <= m || maxSeen.CompareAndSwap(m, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	st := storage.NewMemStorage()
	for i := 0; i < 30; i++ {
		v := float64(i)
		st.SetGauge(context.Background(), fmt.Sprintf("g%d", i), &v)
	}

	a := &Agent{
		Storage:        st,
		Client:         resty.NewWithClient(ts.Client()),
		Config:         config.Config{Address: config.Address(ts.URL), RateLimitInt64: rateLimit},
		PollInterval:   time.Hour,
		ReportInterval: 10 * time.Millisecond,
		Logger:         zap.NewNop(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = a.Run(ctx)

	if got := maxSeen.Load(); got > rateLimit {
		t.Fatalf("одновременных исходящих запросов = %d, превышает лимит %d", got, rateLimit)
	}
	if maxSeen.Load() == 0 {
		t.Fatal("ни одного запроса не отправлено")
	}
}

func TestNew(t *testing.T) {
	st := storage.NewMemStorage()

	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Sync()

	got := New(st, logger)

	if got.Storage != st {
		t.Fatal("экземпляр хранилища не присвоен")
	}
	if got.Client == nil {
		t.Fatal("клиент равен nil")
	}
}
