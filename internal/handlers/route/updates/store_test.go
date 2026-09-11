package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

func sampleBatch() []metrics.Metrics {
	batch := make([]metrics.Metrics, 0, 31)
	for i := range 30 {
		v := float64(i*1000) + 0.5
		batch = append(batch, metrics.Metrics{ID: fmt.Sprintf("Gauge%02d", i), MType: metrics.Gauge, Value: &v})
	}
	delta := int64(42)
	return append(batch, metrics.Metrics{ID: "PollCount", MType: metrics.Counter, Delta: &delta})
}

func TestStoreApplicationJSONBatch(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		assert     func(t *testing.T, st *storage.MemStorage, rr *httptest.ResponseRecorder)
	}{
		{
			name:       "обновляет gauge и counter",
			body:       `[{"id":"alloc","type":"gauge","value":12.5},{"id":"requests","type":"counter","delta":7}]`,
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, st *storage.MemStorage, rr *httptest.ResponseRecorder) {
				t.Helper()
				ctx := context.Background()
				if g := st.GetGauges(ctx)["alloc"]; g == nil || *g != 12.5 {
					t.Fatalf("gauge alloc = %v, ожидается 12.5", g)
				}
				if c := st.GetCounters(ctx)["requests"]; c == nil || *c != 7 {
					t.Fatalf("counter requests = %v, ожидается 7", c)
				}
				if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
					t.Fatalf("Content-Type = %q, ожидается application/json", ct)
				}
				var echo []metrics.Metrics
				if err := json.NewDecoder(rr.Body).Decode(&echo); err != nil {
					t.Fatalf("json.Decode() ошибка = %v", err)
				}
				if len(echo) != 2 {
					t.Fatalf("len(echo) = %d, ожидается 2", len(echo))
				}
			},
		},
		{name: "пустой массив", body: `[]`, wantStatus: http.StatusBadRequest},
		{name: "битый JSON", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "пустое название", body: `[{"id":" ","type":"gauge","value":1}]`, wantStatus: http.StatusNotFound},
		{name: "неизвестный тип", body: `[{"id":"x","type":"histogram","value":1}]`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := storage.NewMemStorage()
			req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			StoreApplicationJSONBatch(st, zap.NewNop()).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("статус = %d, ожидается %d", rr.Code, tt.wantStatus)
			}
			if tt.assert != nil {
				tt.assert(t, st, rr)
			}
		})
	}
}

func BenchmarkStoreApplicationJSONBatch(b *testing.B) {
	body, err := json.Marshal(sampleBatch())
	if err != nil {
		b.Fatalf("json.Marshal() ошибка = %v", err)
	}
	h := StoreApplicationJSONBatch(storage.NewMemStorage(), zap.NewNop())
	rd := bytes.NewReader(body)
	req := httptest.NewRequest(http.MethodPost, "/updates/", rd)
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

func TestStoreApplicationJSONBatchRecordsAuditOnWriteError(t *testing.T) {
	var recorded audit.Names
	ctx := audit.WithRecord(context.Background(), &recorded)
	req := httptest.NewRequest(http.MethodPost, "/updates/",
		strings.NewReader(`[{"id":"Alloc","type":"gauge","value":1}]`)).WithContext(ctx)

	h := StoreApplicationJSONBatch(storage.NewMemStorage(), zap.NewNop())
	h(brokenWriter{httptest.NewRecorder()}, req)

	if got := recorded.Collected(); len(got) != 1 || got[0] != "Alloc" {
		t.Fatalf("имена для аудита = %v, ожидается [Alloc]", got)
	}
}
