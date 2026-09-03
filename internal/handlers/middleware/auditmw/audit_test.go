package auditmw

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/compress"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/hash"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/update"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/updates"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/value"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

const (
	remoteAddr = "10.0.0.5:34567"
	wantIP     = "10.0.0.5"
)

type fakeNotifier struct {
	mu     sync.Mutex
	events []audit.Event
}

func (f *fakeNotifier) Publish(e audit.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
}

func (f *fakeNotifier) all() []audit.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]audit.Event, len(f.events))
	copy(out, f.events)
	return out
}

func newRouter(n audit.Notifier) chi.Router {
	st := storage.NewMemStorage()
	log := zap.NewNop()

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(compress.GzipMiddleware())
		r.Use(hash.Middleware("", log))
		r.With(FromPath(n, log)).Post("/update/{type}/{name}/{value}", update.StoreTextPlain(st))
		r.Get("/value/{type}/{name}", value.ShowTextPlain(st))
		r.With(FromBody(n, log)).Post("/update/", update.StoreApplicationJSON(st))
		r.With(FromBody(n, log)).Post("/updates/", updates.StoreApplicationJSONBatch(st, log))
	})
	return r
}

func assertMetrics(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("метрики в событии = %v, ожидается %v", got, want)
	}
}

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		wantStatus  int
		wantEvents  int
		wantMetrics []string
	}{
		{
			name:        "text/plain: одна метрика из пути",
			method:      http.MethodPost,
			path:        "/update/gauge/Alloc/12.5",
			contentType: "text/plain",
			wantStatus:  http.StatusOK,
			wantEvents:  1,
			wantMetrics: []string{"Alloc"},
		},
		{
			name:        "JSON: одна метрика из тела",
			method:      http.MethodPost,
			path:        "/update/",
			body:        `{"id":"Frees","type":"counter","delta":3}`,
			contentType: "application/json",
			wantStatus:  http.StatusOK,
			wantEvents:  1,
			wantMetrics: []string{"Frees"},
		},
		{
			name:        "JSON: батч из трёх метрик",
			method:      http.MethodPost,
			path:        "/updates/",
			body:        `[{"id":"Alloc","type":"gauge","value":1},{"id":"Frees","type":"counter","delta":2},{"id":"Sys","type":"gauge","value":3}]`,
			contentType: "application/json",
			wantStatus:  http.StatusOK,
			wantEvents:  1,
			wantMetrics: []string{"Alloc", "Frees", "Sys"},
		},
		{
			name:        "битый JSON не порождает событие",
			method:      http.MethodPost,
			path:        "/update/",
			body:        `{`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
			wantEvents:  0,
		},
		{
			name:        "неизвестный тип метрики не порождает событие",
			method:      http.MethodPost,
			path:        "/update/unknown/Alloc/1",
			contentType: "text/plain",
			wantStatus:  http.StatusBadRequest,
			wantEvents:  0,
		},
		{
			name:        "чтение метрики не порождает событие",
			method:      http.MethodGet,
			path:        "/value/gauge/Alloc",
			contentType: "text/plain",
			wantStatus:  http.StatusNotFound,
			wantEvents:  0,
		},
		{
			name:        "имя метрики декодируется из URL",
			method:      http.MethodPost,
			path:        "/update/gauge/My%20Metric/1",
			contentType: "text/plain",
			wantStatus:  http.StatusOK,
			wantEvents:  1,
			wantMetrics: []string{"My Metric"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &fakeNotifier{}
			r := newRouter(n)

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			req.RemoteAddr = remoteAddr

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("статус = %d, ожидается %d (тело %q)", rr.Code, tt.wantStatus, rr.Body.String())
			}

			events := n.all()
			if len(events) != tt.wantEvents {
				t.Fatalf("опубликовано %d событий, ожидается %d", len(events), tt.wantEvents)
			}
			if tt.wantEvents == 0 {
				return
			}

			assertMetrics(t, events[0].Metrics, tt.wantMetrics)
			if events[0].IPAddress != wantIP {
				t.Errorf("ip_address = %q, ожидается %q", events[0].IPAddress, wantIP)
			}
			if events[0].TS <= 0 {
				t.Errorf("ts = %d, ожидается положительный unix timestamp", events[0].TS)
			}
		})
	}
}

func TestMiddlewareReadsGzippedBody(t *testing.T) {
	n := &fakeNotifier{}
	r := newRouter(n)

	payload := `[{"id":"Alloc","type":"gauge","value":1},{"id":"Frees","type":"counter","delta":2}]`

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(payload)); err != nil {
		t.Fatalf("не удалось сжать тело: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("не удалось закрыть gzip-писатель: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.RemoteAddr = remoteAddr

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидается %d (тело %q)", rr.Code, http.StatusOK, rr.Body.String())
	}

	events := n.all()
	if len(events) != 1 {
		t.Fatalf("опубликовано %d событий, ожидается 1", len(events))
	}
	assertMetrics(t, events[0].Metrics, []string{"Alloc", "Frees"})
}

func TestMiddlewareKeepsBodyIntactForHandler(t *testing.T) {
	n := &fakeNotifier{}
	r := newRouter(n)

	payload := `{"id":"Alloc","type":"gauge","value":12.5}`
	req := httptest.NewRequest(http.MethodPost, "/update/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидается %d", rr.Code, http.StatusOK)
	}

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("ответ %q не разбирается: %v", rr.Body.String(), err)
	}
	if got["id"] != "Alloc" || got["value"] != 12.5 {
		t.Errorf("хендлер получил повреждённое тело, ответ: %v", got)
	}
}

func TestNamesFromBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    []string
		wantErr bool
	}{
		{name: "объект", body: `{"id":"Alloc","type":"gauge","value":1}`, want: []string{"Alloc"}},
		{name: "массив", body: `[{"id":"Alloc"},{"id":"Frees"}]`, want: []string{"Alloc", "Frees"}},
		{name: "пробелы перед массивом", body: " \n\t[{\"id\":\"Sys\"}]", want: []string{"Sys"}},
		{name: "битый JSON", body: `{`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := namesFromBody(nil, []byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, ожидается ошибка: %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				assertMetrics(t, got, tt.want)
			}
		})
	}
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("поток повреждён") }
func (brokenBody) Close() error             { return nil }

func TestReadBody(t *testing.T) {
	t.Run("превышение лимита даёт 413", func(t *testing.T) {
		body := strings.Repeat("a", maxBodySize+1)
		req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
		rr := httptest.NewRecorder()
		if _, ok := readBody(rr, req); ok || rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("ok = %v, статус = %d; ожидается false, %d", ok, rr.Code, http.StatusRequestEntityTooLarge)
		}
	})

	t.Run("повреждённое тело даёт 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/updates/", brokenBody{})
		rr := httptest.NewRecorder()
		if _, ok := readBody(rr, req); ok || rr.Code != http.StatusBadRequest {
			t.Fatalf("ok = %v, статус = %d; ожидается false, %d", ok, rr.Code, http.StatusBadRequest)
		}
	})
}
