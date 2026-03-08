package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Shigabutdinoff/metrics/internal/model/metrics"
	"github.com/Shigabutdinoff/metrics/internal/storage"
)

type stubStorage struct {
	gauges   storage.Gauges
	counters storage.Counters
}

func (s *stubStorage) SetGauge(name string, value metrics.GaugeValue) {
	if s.gauges == nil {
		s.gauges = make(storage.Gauges)
	}
	s.gauges[name] = value
}

func (s *stubStorage) AddCounter(name string, delta metrics.CounterValue) {
	if s.counters == nil {
		s.counters = make(storage.Counters)
	}
	s.counters[name] = delta
}

func (s *stubStorage) GetGauges() storage.Gauges {
	if s.gauges == nil {
		return make(storage.Gauges)
	}
	return s.gauges
}

func (s *stubStorage) GetCounters() storage.Counters {
	if s.counters == nil {
		return make(storage.Counters)
	}
	return s.counters
}

func TestIndex(t *testing.T) {
	tests := []struct {
		name            string
		buildStorage    func() storage.Storage
		wantStatus      int
		wantContentType string
		wantBody        string
	}{
		{
			name: "returns placeholder when storage is empty",
			buildStorage: func() storage.Storage {
				return storage.NewMemStorage()
			},
			wantStatus:      http.StatusOK,
			wantContentType: "text/html; charset=utf-8",
			wantBody:        "<html><body><ul><li>No metrics yet</li></ul></body></html>",
		},
		{
			name: "returns sorted and escaped metrics",
			buildStorage: func() storage.Storage {
				st := storage.NewMemStorage()
				gValue := 1.5
				st.SetGauge("b<unsafe>", metrics.GaugeValue(&gValue))

				cValue := int64(2)
				st.AddCounter("a&hits", metrics.CounterValue(&cValue))
				return st
			},
			wantStatus:      http.StatusOK,
			wantContentType: "text/html; charset=utf-8",
			wantBody: "<html><body><ul>" +
				"<li>counter a&amp;hits: 2</li>" +
				"<li>gauge b&lt;unsafe&gt;: 1.5</li>" +
				"</ul></body></html>",
		},
		{
			name: "skips nil metric values",
			buildStorage: func() storage.Storage {
				return &stubStorage{
					gauges: storage.Gauges{
						"nil-gauge": nil,
					},
					counters: storage.Counters{
						"nil-counter": nil,
					},
				}
			},
			wantStatus:      http.StatusOK,
			wantContentType: "text/html; charset=utf-8",
			wantBody:        "<html><body><ul><li>No metrics yet</li></ul></body></html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := tt.buildStorage()

			handler := Index(st)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if got := rr.Header().Get("Content-Type"); got != tt.wantContentType {
				t.Fatalf("content-type = %q, want %q", got, tt.wantContentType)
			}
			if rr.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
		})
	}
}
