package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	config "github.com/shigabutdinoff/metrics/internal/config/agent"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/shigabutdinoff/metrics/pkg/rsacrypt"
)

func TestAgent_CollectMetrics(t *testing.T) {
	st := storage.NewMemStorage()
	a := &Agent{Storage: st}

	a.CollectMetrics()

	poll := st.GetCounters(t.Context())["PollCount"]
	if poll == nil {
		t.Fatal("счётчик PollCount не установлен")
	}
	if *poll != 1 {
		t.Fatalf("PollCount = %d, ожидается 1", *poll)
	}

	randomValue := st.GetGauges(t.Context())["RandomValue"]
	if randomValue == nil {
		t.Fatal("gauge RandomValue не установлен")
	}

	alloc := st.GetGauges(t.Context())["Alloc"]
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
				zr, err := gzip.NewReader(r.Body)
				if err != nil {
					t.Errorf("gzip.NewReader() ошибка = %v", err)
					return
				}
				defer zr.Close()

				body, err := io.ReadAll(zr)
				if err != nil {
					t.Errorf("io.ReadAll() ошибка = %v", err)
					return
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
			if err := a.sendMetrics(t.Context(), items); err != nil {
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

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
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

func TestAgent_Run_GRPC(t *testing.T) {
	var httpRequests atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpRequests.Add(1)
	}))
	defer ts.Close()
	f := &fakeMetricsServer{}
	core, logs := observer.New(zap.WarnLevel)
	a := &Agent{
		Storage:        storage.NewMemStorage(),
		Client:         resty.NewWithClient(ts.Client()),
		Config:         config.Config{Address: config.Address(ts.URL), GRPCAddress: startGRPC(t, f), Key: "k", CryptoKey: testCert},
		PollInterval:   20 * time.Millisecond,
		ReportInterval: 50 * time.Millisecond,
		Logger:         zap.New(core),
	}
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	if err := a.Run(ctx); err != nil {
		t.Fatalf("Run() ошибка = %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reqs) == 0 {
		t.Fatal("ни одной пачки по gRPC не отправлено")
	}
	if !slices.Equal(f.realIPs[0], []string{"127.0.0.1"}) {
		t.Fatalf("x-real-ip = %q, ожидается 127.0.0.1", f.realIPs[0])
	}
	if n := httpRequests.Load(); n != 0 {
		t.Fatalf("запросов по HTTP = %d, ожидается 0", n)
	}
	if n := logs.FilterMessageSnippet("Подпись по gRPC не применяется").Len(); n != 1 {
		t.Fatalf("предупреждений о подписи = %d, ожидается 1", n)
	}
}

func TestAgent_CollectGopsutilMetrics(t *testing.T) {
	st := storage.NewMemStorage()
	a := &Agent{Storage: st, Logger: zap.NewNop()}

	a.collectGopsutil(t.Context())

	gauges := st.GetGauges(t.Context())

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
		st.SetGauge(t.Context(), fmt.Sprintf("g%d", i), &v)
	}

	a := &Agent{
		Storage:        st,
		Client:         resty.NewWithClient(ts.Client()),
		Config:         config.Config{Address: config.Address(ts.URL), RateLimitInt64: rateLimit},
		PollInterval:   time.Hour,
		ReportInterval: 10 * time.Millisecond,
		Logger:         zap.NewNop(),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
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

func TestAgent_BuildBatch(t *testing.T) {
	st := storage.NewMemStorage()
	ctx := t.Context()
	g1, g2 := 1.5, 2.5
	st.SetGauge(ctx, "g1", &g1)
	st.SetGauge(ctx, "g2", &g2)
	st.SetGauge(ctx, "nil", nil)
	delta := int64(7)
	st.AddCounter(ctx, "PollCount", &delta)

	a := &Agent{Storage: st}
	batch := a.buildBatch(ctx)

	if len(batch) != 3 {
		t.Fatalf("len(batch) = %d, ожидается 3 (nil-значение пропускается)", len(batch))
	}
	for _, m := range batch {
		if m.ID == "nil" {
			t.Fatal("метрика с nil-значением попала в батч")
		}
	}
}

func TestAgent_SendMetrics_RealIP(t *testing.T) {
	var got []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Values("X-Real-IP")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	g := 1.5
	a := &Agent{
		Storage: storage.NewMemStorage(),
		Client:  resty.NewWithClient(ts.Client()),
		Config:  config.Config{Address: config.Address(ts.URL)},
	}

	items := []metrics.Metrics{{ID: "cpu", MType: metrics.Gauge, Value: &g}}
	if err := a.sendMetrics(t.Context(), items); err != nil {
		t.Fatalf("sendMetrics() ошибка = %v", err)
	}

	if !slices.Equal(got, []string{"127.0.0.1"}) {
		t.Fatalf("X-Real-IP = %q, ожидается 127.0.0.1", got)
	}
}

func TestHostIP(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		want    string
		wantErr bool
	}{
		{name: "адрес с портом", cfg: config.Config{Address: "http://127.0.0.1:8080"}, want: "127.0.0.1"},
		{name: "адрес без порта", cfg: config.Config{Address: "http://127.0.0.1"}, want: "127.0.0.1"},
		{name: "localhost", cfg: config.Config{Address: "http://localhost:8080"}, want: "127.0.0.1"},
		{name: "неразбираемый адрес", cfg: config.Config{Address: "http://[::1"}, wantErr: true},
		{name: "адрес gRPC", cfg: config.Config{Address: "http://[::1", GRPCAddress: "127.0.0.1:3200"}, want: "127.0.0.1"},
		{name: "адрес gRPC со схемой", cfg: config.Config{GRPCAddress: "dns:///127.0.0.1:3200"}, want: "127.0.0.1"},
		{name: "адрес gRPC без порта", cfg: config.Config{GRPCAddress: "127.0.0.1"}, want: "127.0.0.1"},
		{name: "адрес gRPC со схемой без слешей", cfg: config.Config{GRPCAddress: "dns:127.0.0.1:3200"}, want: "127.0.0.1"},
		{name: "адрес gRPC в скобках без порта", cfg: config.Config{GRPCAddress: "[127.0.0.1]"}, want: "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{Config: tt.cfg}
			ip, err := a.hostIP(t.Context())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("hostIP() = %v, ожидается ошибка", ip)
				}
				return
			}
			if err != nil {
				t.Fatalf("hostIP() ошибка = %v", err)
			}
			if ip.String() != tt.want {
				t.Fatalf("hostIP() = %q, ожидается %q", ip, tt.want)
			}
		})
	}
}

func TestAgent_SendMetrics_Reuse(t *testing.T) {
	var received atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Errorf("gzip.NewReader() ошибка = %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer zr.Close()
		var items []metrics.Metrics
		if err := json.NewDecoder(zr).Decode(&items); err != nil {
			t.Errorf("json.Decode() ошибка = %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received.Add(int64(len(items)))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a := &Agent{
		Storage: storage.NewMemStorage(),
		Client:  resty.NewWithClient(ts.Client()),
		Config:  config.Config{Address: config.Address(ts.URL), Key: "secret"},
	}

	items := make([]metrics.Metrics, 3)
	for i := range items {
		v := float64(i)
		items[i] = metrics.Metrics{ID: fmt.Sprintf("g%d", i), MType: metrics.Gauge, Value: &v}
	}

	const goroutines, sends = 8, 5
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			for range sends {
				if err := a.sendMetrics(t.Context(), items); err != nil {
					t.Errorf("горутина %d: sendMetrics() ошибка = %v", g, err)
					return
				}
			}
		})
	}
	wg.Wait()

	want := int64(goroutines * sends * len(items))
	if got := received.Load(); got != want {
		t.Fatalf("сервер принял %d метрик, ожидается %d", got, want)
	}
}

func BenchmarkAgent_CollectMetrics(b *testing.B) {
	a := &Agent{Storage: storage.NewMemStorage()}

	b.ReportAllocs()
	for b.Loop() {
		a.CollectMetrics()
	}
}

func BenchmarkAgent_BuildBatch(b *testing.B) {
	a := &Agent{Storage: storage.NewMemStorage()}
	a.CollectMetrics()
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		a.buildBatch(ctx)
	}
}

func BenchmarkAgent_SendMetrics(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a := &Agent{
		Storage: storage.NewMemStorage(),
		Client:  resty.NewWithClient(ts.Client()),
		Config:  config.Config{Address: config.Address(ts.URL), Key: "secret"},
	}
	a.CollectMetrics()
	ctx := context.Background()
	items := a.buildBatch(ctx)

	b.ReportAllocs()
	for b.Loop() {
		if err := a.sendMetrics(ctx, items); err != nil {
			b.Fatalf("sendMetrics() ошибка = %v", err)
		}
	}
}

func TestAgent_SendMetrics_Encrypted(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	var got []metrics.Metrics
	var gotHash, wantHash string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encrypted, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("io.ReadAll() ошибка = %v", err)
			return
		}
		compressed, err := rsacrypt.Decrypt(key, encrypted)
		if err != nil {
			t.Errorf("rsacrypt.Decrypt() ошибка = %v", err)
			return
		}
		zr, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Errorf("gzip.NewReader() ошибка = %v", err)
			return
		}
		defer zr.Close()
		body, err := io.ReadAll(zr)
		if err != nil {
			t.Errorf("io.ReadAll(gzip) ошибка = %v", err)
			return
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("json.Unmarshal() ошибка = %v", err)
			return
		}
		mac := hmac.New(sha256.New, []byte("secret"))
		mac.Write(body)
		wantHash = hex.EncodeToString(mac.Sum(nil))
		gotHash = r.Header.Get("HashSHA256")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	g := 1.5
	a := &Agent{
		Storage:   storage.NewMemStorage(),
		Client:    resty.NewWithClient(ts.Client()),
		Config:    config.Config{Address: config.Address(ts.URL), Key: "secret"},
		PublicKey: &key.PublicKey,
	}

	items := []metrics.Metrics{{ID: "cpu", MType: metrics.Gauge, Value: &g}}
	if err := a.sendMetrics(t.Context(), items); err != nil {
		t.Fatalf("sendMetrics() ошибка = %v", err)
	}

	if len(got) != 1 || got[0].ID != "cpu" || got[0].Value == nil || *got[0].Value != g {
		t.Fatalf("сервер получил %+v, ожидается %+v", got, items)
	}
	if gotHash != wantHash {
		t.Fatalf("HashSHA256 = %q, ожидается %q по открытому JSON", gotHash, wantHash)
	}
}

func TestAgent_Run_BadCryptoKey(t *testing.T) {
	a := &Agent{
		Storage: storage.NewMemStorage(),
		Config:  config.Config{CryptoKey: filepath.Join(t.TempDir(), "missing.pem")},
		Logger:  zap.NewNop(),
	}

	if err := a.Run(t.Context()); err == nil {
		t.Fatal("ожидалась ошибка загрузки публичного ключа")
	}
}

func TestAgent_Run_GRPCWithoutCert(t *testing.T) {
	a := &Agent{
		Storage: storage.NewMemStorage(),
		Config:  config.Config{GRPCAddress: "127.0.0.1:3200"},
		Logger:  zap.NewNop(),
	}

	if err := a.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "флаг -crypto-key") {
		t.Fatalf("Run() ошибка = %v, ожидается ошибка клиента gRPC", err)
	}
}

func TestAgent_Run_DeliversQueueAfterCancel(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var delivered, aborted atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Обрыв соединения сервер замечает только после вычитанного тела.
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
			delivered.Add(1)
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			aborted.Add(1)
		}
	}))
	defer ts.Close()

	st := storage.NewMemStorage()
	v := 1.0
	st.SetGauge(t.Context(), "g", &v)

	a := &Agent{
		Storage:        st,
		Client:         resty.NewWithClient(ts.Client()),
		Config:         config.Config{Address: config.Address(ts.URL), RateLimitInt64: 1},
		PollInterval:   time.Hour,
		ReportInterval: 10 * time.Millisecond,
		Logger:         zap.NewNop(),
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- a.Run(ctx) }()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("первый запрос не начался")
	}
	// Воркер занят первым запросом: вторая пачка в очереди, третья ждёт места.
	time.Sleep(100 * time.Millisecond)
	cancel()
	// Отмена успевает дойти до клиента, если отправка от неё зависит.
	time.Sleep(100 * time.Millisecond)
	close(release)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() ошибка = %v, ожидается nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился после отмены контекста")
	}

	if got := aborted.Load(); got != 0 {
		t.Fatalf("оборвано запросов = %d, ожидается 0", got)
	}
	if got := delivered.Load(); got < 3 {
		t.Fatalf("доставлено запросов = %d, ожидается не меньше 3", got)
	}
}

func TestAgent_Run_ShutdownTimeout(t *testing.T) {
	started := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case started <- struct{}{}:
		default:
		}
		// Зависший сервер: ответа нет, пока клиент не оборвёт запрос.
		<-r.Context().Done()
	}))
	defer ts.Close()

	st := storage.NewMemStorage()
	v := 1.0
	st.SetGauge(t.Context(), "g", &v)

	core, logs := observer.New(zap.WarnLevel)
	a := &Agent{
		Storage:         st,
		Client:          resty.NewWithClient(ts.Client()),
		Config:          config.Config{Address: config.Address(ts.URL), RateLimitInt64: 1},
		PollInterval:    time.Hour,
		ReportInterval:  10 * time.Millisecond,
		Logger:          zap.New(core),
		shutdownTimeout: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- a.Run(ctx) }()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("первый запрос не начался")
	}
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() ошибка = %v, ожидается nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился после таймаута досылки")
	}

	if got := logs.FilterMessageSnippet("Очередь метрик не отправлена").Len(); got != 1 {
		t.Fatalf("предупреждений о таймауте = %d, ожидается 1", got)
	}
}
