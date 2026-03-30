package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

func TestStoreTextPlain(t *testing.T) {
	tests := []struct {
		name       string
		typeValue  string
		nameValue  string
		rawValue   string
		wantStatus int
		wantBody   string
		assert     func(t *testing.T, st *storage.MemStorage)
	}{
		{
			name:       "updates gauge",
			typeValue:  "gauge",
			nameValue:  "alloc",
			rawValue:   "12.5",
			wantStatus: http.StatusOK,
			wantBody:   "OK",
			assert: func(t *testing.T, st *storage.MemStorage) {
				t.Helper()
				g := st.GetGauges(context.Background())["alloc"]
				if g == nil || *g != 12.5 {
					t.Fatalf("gauge alloc = %v, want 12.5", g)
				}
			},
		},
		{
			name:       "updates counter",
			typeValue:  "counter",
			nameValue:  "requests",
			rawValue:   "7",
			wantStatus: http.StatusOK,
			wantBody:   "OK",
			assert: func(t *testing.T, st *storage.MemStorage) {
				t.Helper()
				c := st.GetCounters(context.Background())["requests"]
				if c == nil || *c != 7 {
					t.Fatalf("counter requests = %v, want 7", c)
				}
			},
		},
		{
			name:       "returns bad request on unknown type",
			typeValue:  "histogram",
			nameValue:  "x",
			rawValue:   "1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns not found on empty name",
			typeValue:  "gauge",
			nameValue:  "%20",
			rawValue:   "1",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := storage.NewMemStorage()
			r := chi.NewRouter()
			r.Post("/update/{type}/{name}/{value}", StoreTextPlain(st))

			req := httptest.NewRequest(
				http.MethodPost,
				"/update/"+tt.typeValue+"/"+tt.nameValue+"/"+tt.rawValue,
				nil,
			)
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
			if tt.assert != nil {
				tt.assert(t, st)
			}
		})
	}
}
