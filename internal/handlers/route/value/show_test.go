package value

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Shigabutdinoff/metrics/internal/model/metrics"
	"github.com/Shigabutdinoff/metrics/internal/storage"
	"github.com/go-chi/chi/v5"
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
			r.Get("/value/{type}/{name}", Show(st))

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
