package value

import (
	"bytes"
	"encoding/json"
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
			name:      "returns gauge value",
			typeValue: "gauge",
			nameValue: "alloc",
			prepare: func(st *storage.MemStorage) {
				v := 12.5
				st.SetGauge("alloc", metrics.GaugeValue(&v))
			},
			wantStatus: http.StatusOK,
			wantBody:   "12.5",
		},
		{
			name:      "returns counter value",
			typeValue: "counter",
			nameValue: "requests",
			prepare: func(st *storage.MemStorage) {
				v := int64(7)
				st.AddCounter("requests", metrics.CounterValue(&v))
			},
			wantStatus: http.StatusOK,
			wantBody:   "7",
		},
		{
			name:       "returns bad request on unknown type",
			typeValue:  "histogram",
			nameValue:  "x",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns not found on empty name",
			typeValue:  "gauge",
			nameValue:  "%20",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "returns not found for absent metric",
			typeValue:  "counter",
			nameValue:  "absent",
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
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", rr.Body.String(), tt.wantBody)
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
			name: "returns gauge metric as json",
			prepare: func(st *storage.MemStorage) {
				v := 12.5
				st.SetGauge("alloc", metrics.GaugeValue(&v))
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
			name: "returns counter metric as json",
			prepare: func(st *storage.MemStorage) {
				v := int64(7)
				st.AddCounter("requests", metrics.CounterValue(&v))
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
			name: "returns not found for absent metric",
			requestBody: metrics.Metrics{
				ID:    "absent",
				MType: metrics.Counter,
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "returns bad request on unknown type",
			requestBody: metrics.Metrics{
				ID:    "alloc",
				MType: metrics.Type("histogram"),
			},
			wantStatus: http.StatusBadRequest,
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
				t.Fatalf("json.Marshal() error = %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/value", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			ShowApplicationJSON(st).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if tt.wantStatus != http.StatusOK {
				return
			}

			if got := rr.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want %q", got, "application/json")
			}

			var resp metrics.Metrics
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("json.Decode() error = %v", err)
			}

			if resp.ID != tt.wantResponse.ID {
				t.Fatalf("ID = %q, want %q", resp.ID, tt.wantResponse.ID)
			}
			if resp.MType != tt.wantResponse.MType {
				t.Fatalf("MType = %q, want %q", resp.MType, tt.wantResponse.MType)
			}
			if !equalFloat64Ptr(resp.Value, tt.wantResponse.Value) {
				t.Fatalf("Value = %v, want %v", resp.Value, tt.wantResponse.Value)
			}
			if !equalInt64Ptr(resp.Delta, tt.wantResponse.Delta) {
				t.Fatalf("Delta = %v, want %v", resp.Delta, tt.wantResponse.Delta)
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
