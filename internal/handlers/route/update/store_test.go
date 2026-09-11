package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
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
			name:       "обновляет gauge",
			typeValue:  "gauge",
			nameValue:  "alloc",
			rawValue:   "12.5",
			wantStatus: http.StatusOK,
			wantBody:   "OK",
			assert: func(t *testing.T, st *storage.MemStorage) {
				t.Helper()
				g := st.GetGauges(context.Background())["alloc"]
				if g == nil || *g != 12.5 {
					t.Fatalf("gauge alloc = %v, ожидается 12.5", g)
				}
			},
		},
		{
			name:       "обновляет counter",
			typeValue:  "counter",
			nameValue:  "requests",
			rawValue:   "7",
			wantStatus: http.StatusOK,
			wantBody:   "OK",
			assert: func(t *testing.T, st *storage.MemStorage) {
				t.Helper()
				c := st.GetCounters(context.Background())["requests"]
				if c == nil || *c != 7 {
					t.Fatalf("counter requests = %v, ожидается 7", c)
				}
			},
		},
		{
			name:       "возвращает bad request при неизвестном типе",
			typeValue:  "histogram",
			nameValue:  "x",
			rawValue:   "1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "возвращает not found при пустом названии",
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
				t.Fatalf("статус = %d, ожидается %d", rr.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Fatalf("тело = %q, ожидается %q", rr.Body.String(), tt.wantBody)
			}
			if tt.assert != nil {
				tt.assert(t, st)
			}
		})
	}
}

func TestStoreApplicationJSON(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		assert     func(t *testing.T, st *storage.MemStorage, rr *httptest.ResponseRecorder)
	}{
		{
			name:       "обновляет gauge",
			body:       `{"id":"alloc","type":"gauge","value":12.5}`,
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, st *storage.MemStorage, rr *httptest.ResponseRecorder) {
				t.Helper()
				if g := st.GetGauges(context.Background())["alloc"]; g == nil || *g != 12.5 {
					t.Fatalf("gauge alloc = %v, ожидается 12.5", g)
				}
				if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
					t.Fatalf("Content-Type = %q, ожидается application/json", ct)
				}
				var echo metrics.Metrics
				if err := json.NewDecoder(rr.Body).Decode(&echo); err != nil {
					t.Fatalf("json.Decode() ошибка = %v", err)
				}
				if echo.ID != "alloc" || echo.Value == nil || *echo.Value != 12.5 {
					t.Fatalf("echo = %+v, ожидается alloc=12.5", echo)
				}
			},
		},
		{
			name:       "обновляет counter",
			body:       `{"id":"requests","type":"counter","delta":7}`,
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, st *storage.MemStorage, _ *httptest.ResponseRecorder) {
				t.Helper()
				if c := st.GetCounters(context.Background())["requests"]; c == nil || *c != 7 {
					t.Fatalf("counter requests = %v, ожидается 7", c)
				}
			},
		},
		{name: "битый JSON", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "пустое название", body: `{"id":" ","type":"gauge","value":1}`, wantStatus: http.StatusNotFound},
		{name: "неизвестный тип", body: `{"id":"x","type":"histogram","value":1}`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := storage.NewMemStorage()
			req := httptest.NewRequest(http.MethodPost, "/update/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			StoreApplicationJSON(st).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("статус = %d, ожидается %d", rr.Code, tt.wantStatus)
			}
			if tt.assert != nil {
				tt.assert(t, st, rr)
			}
		})
	}
}

func BenchmarkStoreTextPlain(b *testing.B) {
	r := chi.NewRouter()
	r.Post("/update/{type}/{name}/{value}", StoreTextPlain(storage.NewMemStorage()))
	req := httptest.NewRequest(http.MethodPost, "/update/gauge/Alloc/2457600.5", nil)

	b.ReportAllocs()
	for b.Loop() {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			b.Fatalf("статус = %d, ожидается %d", rr.Code, http.StatusOK)
		}
	}
}

func BenchmarkStoreApplicationJSON(b *testing.B) {
	body := []byte(`{"id":"Alloc","type":"gauge","value":2457600.5}`)
	h := StoreApplicationJSON(storage.NewMemStorage())
	rd := bytes.NewReader(body)
	req := httptest.NewRequest(http.MethodPost, "/update/", rd)
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

type brokenWriter struct {
	http.ResponseWriter
}

func (brokenWriter) Write([]byte) (int, error) {
	return 0, errors.New("соединение закрыто")
}

func TestStoreApplicationJSONRecordsAuditOnWriteError(t *testing.T) {
	var recorded audit.Names
	ctx := audit.WithRecord(context.Background(), &recorded)
	req := httptest.NewRequest(http.MethodPost, "/update/",
		strings.NewReader(`{"id":"Alloc","type":"gauge","value":1}`)).WithContext(ctx)

	StoreApplicationJSON(storage.NewMemStorage())(brokenWriter{httptest.NewRecorder()}, req)

	if got := recorded.Collected(); len(got) != 1 || got[0] != "Alloc" {
		t.Fatalf("имена для аудита = %v, ожидается [Alloc]", got)
	}
}
