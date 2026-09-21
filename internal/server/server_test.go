package server

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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/compress"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/reqbody"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/service/persistent"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/shigabutdinoff/metrics/pkg/jsonconfig"
	"github.com/shigabutdinoff/metrics/pkg/rsacrypt"
)

func TestNew(t *testing.T) {
	st := storage.NewMemStorage()

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	s := New(st, logger)
	s.setupRoutes()

	if s.Storage != st {
		t.Fatalf("New() несоответствие хранилища")
	}
	if s.Address != DefaultAddress {
		t.Fatalf("New() адрес = %q, ожидается %q", s.Address, DefaultAddress)
	}
	if s.Router == nil {
		t.Fatalf("setupRoutes() роутер равен nil")
	}

	t.Run("GET /", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("GET / статус = %d, ожидается %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("POST /update/gauge", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/update/gauge/temp/12.5", nil)
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("POST /update/gauge статус = %d, ожидается %d", rr.Code, http.StatusOK)
		}

		val := st.GetGauges(context.Background())["temp"]
		if val == nil || *val != 12.5 {
			t.Fatalf("gauge temp не обновлён, получено %v", val)
		}
	})

	t.Run("GET /value/gauge", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/value/gauge/temp", nil)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("GET /value/gauge статус = %d, ожидается %d", rr.Code, http.StatusOK)
		}
		if body := rr.Body.String(); body != "12.5" {
			t.Fatalf("GET /value/gauge тело = %q, ожидается %q", body, "12.5")
		}
	})
}

func TestServer_Run(t *testing.T) {
	t.Run("возвращает ошибку прослушивания", func(t *testing.T) {
		s := New(storage.NewMemStorage(), zap.NewNop())
		s.Address = "bad"

		require.Error(t, s.Run(t.Context()))
	})

	t.Run("возвращает ошибку аудита", func(t *testing.T) {
		s := New(storage.NewMemStorage(), zap.NewNop())
		s.Address = "bad"
		s.AuditFile = filepath.Join(t.TempDir(), "missing", "audit.log")

		require.ErrorIs(t, s.Run(t.Context()), os.ErrNotExist)
	})

	t.Run("возвращает ошибку приватного ключа", func(t *testing.T) {
		s := New(storage.NewMemStorage(), zap.NewNop())
		s.Address = "bad"
		s.CryptoKey = filepath.Join(t.TempDir(), "missing.pem")

		require.ErrorIs(t, s.Run(t.Context()), os.ErrNotExist)
	})
}

// Без keep-alive: запасное соединение без запроса держит Shutdown до 5 с.
var testClient = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

func freeAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func newFileServer(t *testing.T, interval jsonconfig.Seconds) *Server {
	t.Helper()

	s := New(storage.NewMemStorage(), zap.NewNop())
	s.FileStoragePath = filepath.Join(t.TempDir(), "metrics.json")
	s.Restore = false
	s.StoreInterval = interval
	return s
}

func runServer(t *testing.T, s *Server) (context.CancelFunc, <-chan error) {
	t.Helper()

	s.Address = freeAddr(t)
	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		errCh <- s.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})

	require.Eventually(t, func() bool {
		if len(errCh) > 0 {
			return true
		}
		resp, err := testClient.Get("http://" + s.Address + "/")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond, "сервер не поднялся")
	if len(errCh) > 0 {
		t.Fatalf("Run завершился до старта сервера: %v", <-errCh)
	}

	return cancel, errCh
}

func waitRun(t *testing.T, errCh <-chan error) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Run не завершился после отмены контекста")
		return nil
	}
}

func postOK(t *testing.T, s *Server, route string) {
	t.Helper()

	resp, err := testClient.Post("http://"+s.Address+route, "text/plain", nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func loadGauge(t *testing.T, path, name string) (*float64, error) {
	t.Helper()

	st := storage.NewMemStorage()
	if err := persistent.New(st, path, zap.NewNop()).Load(); err != nil {
		return nil, err
	}
	return st.GetGauge(t.Context(), name), nil
}

func requireSavedGauge(t *testing.T, path, name string, want float64) {
	t.Helper()

	got, err := loadGauge(t, path, name)
	require.NoError(t, err)
	require.NotNilf(t, got, "gauge %s не сохранён в %s", name, path)
	require.Equal(t, want, *got)
}

func TestServer_Run_SavesOnShutdown(t *testing.T) {
	s := newFileServer(t, 3600)

	cancel, errCh := runServer(t, s)

	postOK(t, s, "/update/gauge/temp/12.5")
	require.NoFileExists(t, s.FileStoragePath)

	cancel()
	require.NoError(t, waitRun(t, errCh))

	requireSavedGauge(t, s.FileStoragePath, "temp", 12.5)
}

func TestServer_Run_FinishesActiveRequest(t *testing.T) {
	s := newFileServer(t, 3600)

	cancel, errCh := runServer(t, s)

	// Тело идёт через pipe: запрос начат, но сервер ждёт его окончания.
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+s.Address+"/update/", pr)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	type result struct {
		status int
		err    error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := testClient.Do(req)
		if err != nil {
			resCh <- result{err: err}
			return
		}
		_ = resp.Body.Close()
		resCh <- result{status: resp.StatusCode}
	}()

	_, err = pw.Write([]byte(`{"id":"temp","type":"gauge",`))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	cancel()
	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-errCh:
		t.Fatalf("Run завершился, не дождавшись активного запроса: %v", err)
	default:
	}

	_, err = pw.Write([]byte(`"value":12.5}`))
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	select {
	case res := <-resCh:
		require.NoError(t, res.err)
		require.Equal(t, http.StatusOK, res.status)
	case <-time.After(5 * time.Second):
		t.Fatal("активный запрос не получил ответ")
	}
	require.NoError(t, waitRun(t, errCh))

	requireSavedGauge(t, s.FileStoragePath, "temp", 12.5)
}

func TestServer_Run_ShutdownTimeout(t *testing.T) {
	s := newFileServer(t, 3600)
	s.shutdownTimeout = 100 * time.Millisecond
	core, logs := observer.New(zap.WarnLevel)
	s.Logger = zap.New(core)

	cancel, errCh := runServer(t, s)

	postOK(t, s, "/update/gauge/temp/12.5")

	// Зависший запрос: заголовки и начало тела без продолжения.
	conn, err := net.Dial("tcp", s.Address)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = io.WriteString(conn, "POST /update/ HTTP/1.1\r\nHost: test\r\n"+
		"Content-Type: application/json\r\nContent-Length: 64\r\n\r\n{\"id\":")
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	cancel()
	require.NoError(t, waitRun(t, errCh))
	require.Equal(t, 1, logs.FilterMessageSnippet("Не удалось штатно остановить сервер").Len())
	requireSavedGauge(t, s.FileStoragePath, "temp", 12.5)

	// После таймаута сервер закрывает соединение, чтение не виснет.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err = io.Copy(io.Discard, conn)
	require.Falsef(t, os.IsTimeout(err), "зависшее соединение не закрыто: %v", err)
}

func TestServer_Run_SaveErrorOnShutdown(t *testing.T) {
	s := newFileServer(t, 3600)

	// Каталог для файла метрик не создать: на его месте обычный файл.
	blocker := filepath.Join(filepath.Dir(s.FileStoragePath), "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o644))
	s.FileStoragePath = filepath.Join(blocker, "metrics.json")

	cancel, errCh := runServer(t, s)
	cancel()

	require.Error(t, waitRun(t, errCh))
}

func TestServer_Run_PeriodicSave(t *testing.T) {
	s := newFileServer(t, 1)
	path := s.FileStoragePath

	cancel, errCh := runServer(t, s)

	postOK(t, s, "/update/gauge/temp/12.5")

	require.Eventually(t, func() bool {
		got, err := loadGauge(t, path, "temp")
		return err == nil && got != nil
	}, 3*time.Second, 50*time.Millisecond, "тикер не сохранил метрики")
	requireSavedGauge(t, path, "temp", 12.5)

	cancel()
	require.NoError(t, waitRun(t, errCh))

	// После остановки тикер не пишет: удалённый файл не появляется снова.
	require.NoError(t, os.Remove(path))
	time.Sleep(1200 * time.Millisecond)
	require.NoFileExists(t, path)
}

func TestServer_Run_SyncSave(t *testing.T) {
	s := newFileServer(t, 0)

	cancel, errCh := runServer(t, s)

	postOK(t, s, "/update/gauge/temp/12.5")

	// Нулевой интервал: метрика в файле сразу после ответа, до остановки.
	requireSavedGauge(t, s.FileStoragePath, "temp", 12.5)

	cancel()
	require.NoError(t, waitRun(t, errCh))
}

func TestServer_Run_ListenErrorKeepsFile(t *testing.T) {
	s := newFileServer(t, 3600)
	s.Address = "bad"
	saved := `[{"id":"temp","type":"gauge","value":12.5}]`
	require.NoError(t, os.WriteFile(s.FileStoragePath, []byte(saved), 0o644))

	require.Error(t, s.Run(t.Context()))

	got, err := os.ReadFile(s.FileStoragePath)
	require.NoError(t, err)
	require.Equal(t, saved, string(got))
}

func TestServer_ConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	data := `{
		"address": "localhost:8081",
		"restore": false,
		"store_interval": "1s",
		"store_file": "/path/to/file.db",
		"database_dsn": "postgres://localhost/metrics",
		"crypto_key": "/path/to/key.pem",
		"key": "secret",
		"audit_file": "/path/to/audit.log",
		"audit_url": "http://localhost:9000/audit",
		"pprof_address": "localhost:6060",
		"database": {}
	}`
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	s := New(storage.NewMemStorage(), zap.NewNop())
	require.NoError(t, jsonconfig.Load(path, s))

	require.Equal(t, "localhost:8081", s.Address)
	require.False(t, s.Restore)
	require.Equal(t, jsonconfig.Seconds(1), s.StoreInterval)
	require.Equal(t, "/path/to/file.db", s.FileStoragePath)
	require.Equal(t, "postgres://localhost/metrics", s.DatabaseDSN)
	require.Equal(t, "/path/to/key.pem", s.CryptoKey)
	require.Equal(t, "secret", s.Key)
	require.Equal(t, "/path/to/audit.log", s.AuditFile)
	require.Equal(t, "http://localhost:9000/audit", s.AuditURL)
	require.Equal(t, "localhost:6060", s.PprofAddress)
	require.Nil(t, s.Database)
}

func TestGzipCompression(t *testing.T) {
	requestBody := `{
        "request": {
            "type": "SimpleUtterance",
            "command": "sudo do something"
        },
        "version": "1.0"
    }`

	successBody := `{
        "response": {
            "text": "Извините, я пока ничего не умею"
        },
        "version": "1.0"
    }`

	handler := compress.GzipMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.JSONEq(t, requestBody, string(body))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(successBody))
		require.NoError(t, err)
	}))

	srv := httptest.NewServer(handler)
	defer srv.Close()

	t.Run("sends_gzip", func(t *testing.T) {
		buf := bytes.NewBuffer(nil)
		zb := gzip.NewWriter(buf)
		_, err := zb.Write([]byte(requestBody))
		require.NoError(t, err)
		err = zb.Close()
		require.NoError(t, err)

		r := httptest.NewRequest("POST", srv.URL, buf)
		r.RequestURI = ""
		r.Header.Set("Content-Encoding", "gzip")
		r.Header.Set("Accept-Encoding", "")

		resp, err := http.DefaultClient.Do(r)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		defer resp.Body.Close()

		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.JSONEq(t, successBody, string(b))
	})

	t.Run("accepts_gzip", func(t *testing.T) {
		buf := bytes.NewBufferString(requestBody)
		r := httptest.NewRequest("POST", srv.URL, buf)
		r.RequestURI = ""
		r.Header.Set("Accept-Encoding", "gzip")

		resp, err := http.DefaultClient.Do(r)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

		defer resp.Body.Close()

		zr, err := gzip.NewReader(resp.Body)
		require.NoError(t, err)

		b, err := io.ReadAll(zr)
		require.NoError(t, err)

		require.JSONEq(t, successBody, string(b))
	})
}

func TestProfiler(t *testing.T) {
	s := New(storage.NewMemStorage(), zap.NewNop())
	s.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)

	h := pprofHandler()
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/cmdline"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		require.Equalf(t, http.StatusOK, rr.Code, "GET %s", path)
		require.NotEmptyf(t, rr.Body.Bytes(), "GET %s: пустое тело", path)
	}
}

func sampleBatch() []metrics.Metrics {
	batch := make([]metrics.Metrics, 0, 31)
	for i := range 30 {
		v := float64(i*1000) + 0.5
		batch = append(batch, metrics.Metrics{ID: fmt.Sprintf("Gauge%02d", i), MType: metrics.Gauge, Value: &v})
	}
	delta := int64(42)
	return append(batch, metrics.Metrics{ID: "PollCount", MType: metrics.Counter, Delta: &delta})
}

func gzipBytes(p []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(p); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func computeHMAC(key, data []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func updatesRequest(gz io.Reader, hash string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/updates/", gz)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("HashSHA256", hash)
	return req
}

func newServer(key string) *Server {
	s := New(storage.NewMemStorage(), zap.NewNop())
	s.Key = key
	s.setupRoutes()
	return s
}

func BenchmarkRouter_Updates(b *testing.B) {
	body, err := json.Marshal(sampleBatch())
	require.NoError(b, err)
	gz := gzipBytes(body)
	hash := computeHMAC([]byte("secret"), body)

	b.Run("gzip+hash", func(b *testing.B) {
		s := newServer("secret")
		rd := bytes.NewReader(gz)
		req := updatesRequest(rd, hash)

		b.ReportAllocs()
		for b.Loop() {
			rd.Reset(gz)
			rr := httptest.NewRecorder()
			s.Router.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				b.Fatalf("статус = %d, ожидается %d", rr.Code, http.StatusOK)
			}
		}
	})

	b.Run("gzip+hash parallel", func(b *testing.B) {
		s := newServer("secret")

		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			rd := bytes.NewReader(gz)
			req := updatesRequest(rd, hash)
			for pb.Next() {
				rd.Reset(gz)
				rr := httptest.NewRecorder()
				s.Router.ServeHTTP(rr, req)
				if rr.Code != http.StatusOK {
					b.Errorf("статус = %d, ожидается %d", rr.Code, http.StatusOK)
					return
				}
			}
		})
	})

	b.Run("plain", func(b *testing.B) {
		s := newServer("")
		rd := bytes.NewReader(body)
		req := httptest.NewRequest(http.MethodPost, "/updates/", rd)
		req.Header.Set("Content-Type", "application/json")

		b.ReportAllocs()
		for b.Loop() {
			rd.Reset(body)
			rr := httptest.NewRecorder()
			s.Router.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				b.Fatalf("статус = %d, ожидается %d", rr.Code, http.StatusOK)
			}
		}
	})
}

func TestCloseAuditClosesFileSink(t *testing.T) {
	s := New(storage.NewMemStorage(), zap.NewNop())
	s.AuditFile = filepath.Join(t.TempDir(), "audit.log")

	require.NoError(t, s.setupAudit())
	require.Len(t, s.auditClosers, 1)
	sink := s.auditClosers[0]

	s.closeAudit()

	require.ErrorIs(t, sink.Close(), os.ErrClosed)
	require.Empty(t, s.auditClosers)
}

func TestSetupAuditFailsFast(t *testing.T) {
	t.Run("файл аудита не открыть", func(t *testing.T) {
		s := New(storage.NewMemStorage(), zap.NewNop())
		s.AuditFile = filepath.Join(t.TempDir(), "missing", "audit.log")

		require.ErrorIs(t, s.setupAudit(), os.ErrNotExist)
		require.Nil(t, s.auditor)
	})

	t.Run("некорректный URL аудита", func(t *testing.T) {
		s := New(storage.NewMemStorage(), zap.NewNop())
		s.AuditURL = "not a url"

		require.Error(t, s.setupAudit())
		require.Nil(t, s.auditor)
	})

	t.Run("файл аудита закрывается при ошибке URL", func(t *testing.T) {
		s := New(storage.NewMemStorage(), zap.NewNop())
		s.Address = "bad"
		s.AuditFile = filepath.Join(t.TempDir(), "audit.log")
		s.AuditURL = "not a url"

		require.Error(t, s.Run(t.Context()))
		require.Nil(t, s.auditor)
		require.Empty(t, s.auditClosers)
	})
}

type recordingSink struct {
	mu     sync.Mutex
	events []audit.Event
}

func (o *recordingSink) Update(_ context.Context, e audit.Event) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, e)
	return nil
}

func (o *recordingSink) snapshot() []audit.Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]audit.Event, len(o.events))
	copy(out, o.events)
	return out
}

func TestRouterPublishesAudit(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		contentType string
		body        string
		want        []string
	}{
		{
			name:        "текстовый роут",
			path:        "/update/gauge/Sys/3",
			contentType: "text/plain",
			want:        []string{"Sys"},
		},
		{
			name:        "json одна метрика",
			path:        "/update/",
			contentType: "application/json",
			body:        `{"id":"Alloc","type":"gauge","value":1}`,
			want:        []string{"Alloc"},
		},
		{
			name:        "json пачка",
			path:        "/updates/",
			contentType: "application/json",
			body:        `[{"id":"Alloc","type":"gauge","value":1},{"id":"Poll","type":"counter","delta":2}]`,
			want:        []string{"Alloc", "Poll"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &recordingSink{}
			p := audit.NewPublisher(zap.NewNop(), audit.WithCloseTimeout(time.Second))
			p.Register(sink)

			s := New(storage.NewMemStorage(), zap.NewNop())
			s.auditor = p
			s.setupRoutes()

			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			rr := httptest.NewRecorder()
			s.Router.ServeHTTP(rr, req)
			require.Equal(t, http.StatusOK, rr.Code)

			p.Close()
			events := sink.snapshot()
			require.Len(t, events, 1)
			require.Equal(t, tt.want, events[0].Metrics)
		})
	}
}

func TestBodyLimit(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte('[')
	buf.Write(bytes.Repeat([]byte(" "), reqbody.MaxBodySize+1))
	buf.WriteString(`{"id":"Alloc","type":"gauge","value":1}]`)
	body := gzipBytes(buf.Bytes())

	for _, key := range []string{"", "secret"} {
		t.Run("ключ="+key, func(t *testing.T) {
			s := newServer(key)

			req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Content-Encoding", "gzip")
			rr := httptest.NewRecorder()
			s.Router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
		})
	}
}

func TestRouter_Updates_Encrypted(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	body, err := json.Marshal(sampleBatch())
	require.NoError(t, err)
	encrypted, err := rsacrypt.Encrypt(&key.PublicKey, gzipBytes(body))
	require.NoError(t, err)
	hash := computeHMAC([]byte("secret"), body)

	tests := []struct {
		name       string
		body       []byte
		wantStatus int
	}{
		{name: "шифрованное тело принимается", body: encrypted, wantStatus: http.StatusOK},
		{name: "открытое тело при заданном ключе отклоняется", body: gzipBytes(body), wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(storage.NewMemStorage(), zap.NewNop())
			s.Key = "secret"
			s.privateKey = key
			s.setupRoutes()

			rr := httptest.NewRecorder()
			s.Router.ServeHTTP(rr, updatesRequest(bytes.NewReader(tt.body), hash))
			require.Equal(t, tt.wantStatus, rr.Code, rr.Body.String())

			poll := s.Storage.GetCounters(t.Context())["PollCount"]
			if tt.wantStatus != http.StatusOK {
				require.Nil(t, poll)
				return
			}
			require.NotNil(t, poll)
			require.EqualValues(t, 42, *poll)
		})
	}
}

func TestIsRetriablePGError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "истёк таймаут", err: context.DeadlineExceeded, want: true},
		{name: "обёрнутая отмена контекста", err: fmt.Errorf("upsert: %w", context.Canceled), want: true},
		{name: "класс 08, сбой соединения", err: errors.New("ERROR: connection failure (SQLSTATE 08006)"), want: true},
		{name: "класс 08 в нижнем регистре", err: errors.New("sqlstate 08001"), want: true},
		{name: "ошибка сериализации", err: errors.New("SQLSTATE 40001"), want: true},
		{name: "взаимная блокировка", err: errors.New("SQLSTATE 40P01"), want: true},
		{name: "остановка сервера БД", err: errors.New("SQLSTATE 57P01"), want: true},
		{name: "слишком много соединений", err: errors.New("SQLSTATE 53300"), want: true},
		{name: "нарушение уникальности", err: errors.New("SQLSTATE 23505"), want: false},
		{name: "ошибка без SQLSTATE", err: errors.New("сбой"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isRetriablePGError(tt.err))
		})
	}
}

func TestWithRetry(t *testing.T) {
	errFatal := errors.New("SQLSTATE 23505")
	errTemp := errors.New("SQLSTATE 08006")

	t.Run("успех с первой попытки", func(t *testing.T) {
		calls := 0
		err := withRetry(func() error { calls++; return nil }, isRetriablePGError, nil)
		require.NoError(t, err)
		require.Equal(t, 1, calls)
	})

	t.Run("неповторяемая ошибка возвращается сразу", func(t *testing.T) {
		calls := 0
		err := withRetry(func() error { calls++; return errFatal }, isRetriablePGError, nil)
		require.ErrorIs(t, err, errFatal)
		require.Equal(t, 1, calls)
	})

	t.Run("повторяемая ошибка, затем успех", func(t *testing.T) {
		core, logs := observer.New(zap.InfoLevel)
		calls := 0
		op := func() error {
			calls++
			if calls == 1 {
				return errTemp
			}
			return nil
		}

		// Первая пауза в withRetry равна секунде.
		require.NoError(t, withRetry(op, isRetriablePGError, zap.New(core)))
		require.Equal(t, 2, calls)

		entries := logs.All()
		require.Len(t, entries, 1)
		require.EqualValues(t, 1, entries[0].ContextMap()["attempt"])
	})
}

func TestServer_Save_NothingConfigured(t *testing.T) {
	s := New(storage.NewMemStorage(), zap.NewNop())
	s.FileStoragePath = ""

	require.NoError(t, s.save(persistent.New(s.Storage, "", s.Logger)))
}
