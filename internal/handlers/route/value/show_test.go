package value

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

func TestShow(t *testing.T) {
	tests := []struct {
		name       string
		typeValue  string
		nameValue  string
		prepare    func(st *storage.MemStorage)
		wantStatus int
		wantBody   string
	}{
		{
			name:      "возвращает значение gauge",
			typeValue: "gauge",
			nameValue: "alloc",
			prepare: func(st *storage.MemStorage) {
				v := 12.5
				st.SetGauge(context.Background(), "alloc", metrics.GaugeValue(&v))
			},
			wantStatus: http.StatusOK,
			wantBody:   "12.5",
		},
		{
			name:      "возвращает значение counter",
			typeValue: "counter",
			nameValue: "requests",
			prepare: func(st *storage.MemStorage) {
				v := int64(7)
				st.AddCounter(context.Background(), "requests", metrics.CounterValue(&v))
			},
			wantStatus: http.StatusOK,
			wantBody:   "7",
		},
		{
			name:       "возвращает bad request при неизвестном типе",
			typeValue:  "histogram",
			nameValue:  "x",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "возвращает not found при пустом названии",
			typeValue:  "gauge",
			nameValue:  "%20",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "возвращает not found для отсутствующей метрики",
			typeValue:  "counter",
			nameValue:  "absent",
			wantStatus: http.StatusNotFound,
		},
		{
			name:      "не путает gauge с одноимённым counter",
			typeValue: "gauge",
			nameValue: "requests",
			prepare: func(st *storage.MemStorage) {
				v := int64(7)
				st.AddCounter(context.Background(), "requests", metrics.CounterValue(&v))
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := storage.NewMemStorage()
			if tt.prepare != nil {
				tt.prepare(st)
			}

			r := chi.NewRouter()
			r.Get("/value/{type}/{name}", ShowTextPlain(st))

			req := httptest.NewRequest(http.MethodGet, "/value/"+tt.typeValue+"/"+tt.nameValue, nil)
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("статус = %d, ожидается %d", rr.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Fatalf("тело = %q, ожидается %q", rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestShowApplicationJSON(t *testing.T) {
	tests := []struct {
		name         string
		prepare      func(st *storage.MemStorage)
		requestBody  metrics.Metrics
		wantStatus   int
		wantResponse metrics.Metrics
	}{
		{
			name: "возвращает метрику gauge в json",
			prepare: func(st *storage.MemStorage) {
				v := 12.5
				st.SetGauge(context.Background(), "alloc", metrics.GaugeValue(&v))
			},
			requestBody: metrics.Metrics{
				ID:    "alloc",
				MType: metrics.Gauge,
			},
			wantStatus: http.StatusOK,
			wantResponse: metrics.Metrics{
				ID:    "alloc",
				MType: metrics.Gauge,
				Value: float64Ptr(12.5),
			},
		},
		{
			name: "возвращает метрику counter в json",
			prepare: func(st *storage.MemStorage) {
				v := int64(7)
				st.AddCounter(context.Background(), "requests", metrics.CounterValue(&v))
			},
			requestBody: metrics.Metrics{
				ID:    "requests",
				MType: metrics.Counter,
			},
			wantStatus: http.StatusOK,
			wantResponse: metrics.Metrics{
				ID:    "requests",
				MType: metrics.Counter,
				Delta: int64Ptr(7),
			},
		},
		{
			name: "возвращает not found для отсутствующей метрики",
			requestBody: metrics.Metrics{
				ID:    "absent",
				MType: metrics.Counter,
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "возвращает bad request при неизвестном типе",
			requestBody: metrics.Metrics{
				ID:    "alloc",
				MType: metrics.Type("histogram"),
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "не путает gauge с одноимённым counter",
			prepare: func(st *storage.MemStorage) {
				v := int64(7)
				st.AddCounter(context.Background(), "requests", metrics.CounterValue(&v))
			},
			requestBody: metrics.Metrics{
				ID:    "requests",
				MType: metrics.Gauge,
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "возвращает not found при пробельном названии",
			requestBody: metrics.Metrics{
				ID:    "  ",
				MType: metrics.Gauge,
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := storage.NewMemStorage()
			if tt.prepare != nil {
				tt.prepare(st)
			}

			body, err := json.Marshal(tt.requestBody)
			if err != nil {
				t.Fatalf("json.Marshal() ошибка = %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/value", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			ShowApplicationJSON(st).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("статус = %d, ожидается %d", rr.Code, tt.wantStatus)
			}

			if tt.wantStatus != http.StatusOK {
				return
			}

			if got := rr.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, ожидается %q", got, "application/json")
			}

			var resp metrics.Metrics
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("json.Decode() ошибка = %v", err)
			}

			if resp.ID != tt.wantResponse.ID {
				t.Fatalf("ID = %q, ожидается %q", resp.ID, tt.wantResponse.ID)
			}
			if resp.MType != tt.wantResponse.MType {
				t.Fatalf("MType = %q, ожидается %q", resp.MType, tt.wantResponse.MType)
			}
			if !equalFloat64Ptr(resp.Value, tt.wantResponse.Value) {
				t.Fatalf("Value = %v, ожидается %v", resp.Value, tt.wantResponse.Value)
			}
			if !equalInt64Ptr(resp.Delta, tt.wantResponse.Delta) {
				t.Fatalf("Delta = %v, ожидается %v", resp.Delta, tt.wantResponse.Delta)
			}
		})
	}
}

func TestShowApplicationJSON_BadBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "битый json", body: `{"id":"alloc",`},
		{name: "пустое тело", body: ""},
		{name: "неверный тип поля", body: `{"id":"alloc","type":"gauge","value":"много"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/value", bytes.NewReader([]byte(tt.body)))
			rr := httptest.NewRecorder()

			ShowApplicationJSON(storage.NewMemStorage()).ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("статус = %d, ожидается %d", rr.Code, http.StatusBadRequest)
			}
		})
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

func equalFloat64Ptr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func fillStorage(st *storage.MemStorage, n int) {
	ctx := context.Background()
	for i := range n {
		st.SetGauge(ctx, fmt.Sprintf("Gauge%02d", i), float64Ptr(float64(i)+0.5))
	}
	st.AddCounter(ctx, "PollCount", int64Ptr(42))
}

func BenchmarkShowTextPlain(b *testing.B) {
	st := storage.NewMemStorage()
	fillStorage(st, 45)
	r := chi.NewRouter()
	r.Get("/value/{type}/{name}", ShowTextPlain(st))

	run := func(b *testing.B, path string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		b.ReportAllocs()
		for b.Loop() {
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				b.Fatalf("статус = %d, ожидается %d", rr.Code, http.StatusOK)
			}
		}
	}

	b.Run("gauge", func(b *testing.B) { run(b, "/value/gauge/Gauge10") })
	b.Run("counter", func(b *testing.B) { run(b, "/value/counter/PollCount") })
}

func BenchmarkShowApplicationJSON(b *testing.B) {
	st := storage.NewMemStorage()
	fillStorage(st, 45)
	h := ShowApplicationJSON(st)
	body := []byte(`{"id":"Gauge10","type":"gauge"}`)
	rd := bytes.NewReader(body)
	req := httptest.NewRequest(http.MethodPost, "/value/", rd)
	req.Header.Set("Content-Type", "application/json")

	b.ReportAllocs()
	for b.Loop() {
		rd.Reset(body)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			b.Fatalf("статус = %d, ожидается %d", rr.Code, http.StatusOK)
		}
	}
}
