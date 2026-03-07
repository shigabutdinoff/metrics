package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shigabutdinoff/metrics/internal/model/metrics"
	"github.com/Shigabutdinoff/metrics/internal/storage"
)

func TestAgent_CollectMetrics(t *testing.T) {
	st := storage.NewMemStorage()
	a := &Agent{Storage: st}

	a.CollectMetrics()

	poll := st.GetCounters()["PollCount"]
	if poll == nil {
		t.Fatal("PollCount counter not set")
	}
	if *poll != 1 {
		t.Fatalf("PollCount = %d, want 1", *poll)
	}

	randomValue := st.GetGauges()["RandomValue"]
	if randomValue == nil {
		t.Fatal("RandomValue gauge not set")
	}

	alloc := st.GetGauges()["Alloc"]
	if alloc == nil {
		t.Fatal("Alloc gauge from runtime stats not set")
	}
}

func TestAgent_ReportMetrics(t *testing.T) {
	st := storage.NewMemStorage()
	g := 12.5
	c := int64(7)
	st.SetGauge("Alloc", &g)
	st.AddCounter("PollCount", &c)

	var received []string
	var methods []string
	var contentTypes []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Method+" "+r.URL.Path)
		methods = append(methods, r.Method)
		contentTypes = append(contentTypes, r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a := &Agent{
		Storage:       st,
		Client:        ts.Client(),
		ServerAddress: ts.URL,
	}

	a.ReportMetrics()

	if len(received) != 2 {
		t.Fatalf("requests count = %d, want 2", len(received))
	}
	for _, method := range methods {
		if method != http.MethodPost {
			t.Fatalf("method = %s, want %s", method, http.MethodPost)
		}
	}
	for _, ct := range contentTypes {
		if !strings.HasPrefix(ct, "text/plain") {
			t.Fatalf("content type = %q, want prefix text/plain", ct)
		}
	}

	paths := strings.Join(received, "\n")
	if !strings.Contains(paths, "/update/gauge/Alloc/12.500000") {
		t.Fatalf("gauge endpoint not called, got %q", paths)
	}
	if !strings.Contains(paths, "/update/counter/PollCount/7") {
		t.Fatalf("counter endpoint not called, got %q", paths)
	}
}

func TestAgent_Run(t *testing.T) {
	t.Skip("Run contains an infinite loop and requires integration-style cancellation")
}

func TestAgent_sendMetric(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		mtype      metrics.Type
		metricName string
		value      any
		wantErr    bool
	}{
		{
			name:       "success gauge",
			statusCode: http.StatusOK,
			mtype:      metrics.Gauge,
			metricName: "Alloc",
			value:      metrics.GaugeValue(float64Ptr(3.5)),
			wantErr:    false,
		},
		{
			name:       "error on non-200",
			statusCode: http.StatusInternalServerError,
			mtype:      metrics.Counter,
			metricName: "PollCount",
			value:      metrics.CounterValue(int64Ptr(3)),
			wantErr:    true,
		},
		{
			name:       "error on invalid value",
			statusCode: http.StatusOK,
			mtype:      metrics.Gauge,
			metricName: "Alloc",
			value:      int64Ptr(2),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer ts.Close()

			a := &Agent{
				Client:        ts.Client(),
				ServerAddress: ts.URL,
			}

			err := a.sendMetric(tt.mtype, tt.metricName, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("sendMetric() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNew(t *testing.T) {
	st := storage.NewMemStorage()
	got := New(st)

	if got.Storage != st {
		t.Fatal("storage instance was not assigned")
	}
	if got.Client == nil {
		t.Fatal("client is nil")
	}
	if got.PollInterval != DefaultPollInterval {
		t.Fatalf("PollInterval = %v, want %v", got.PollInterval, DefaultPollInterval)
	}
	if got.ReportInterval != DefaultReportInterval {
		t.Fatalf("ReportInterval = %v, want %v", got.ReportInterval, DefaultReportInterval)
	}
	if got.ServerAddress != DefaultServerAddress {
		t.Fatalf("ServerAddress = %q, want %q", got.ServerAddress, DefaultServerAddress)
	}
}

func Test_formatMetricValue(t *testing.T) {
	tests := []struct {
		name    string
		mtype   metrics.Type
		key     string
		value   any
		want    string
		wantErr bool
	}{
		{
			name:    "gauge",
			mtype:   metrics.Gauge,
			key:     "Alloc",
			value:   metrics.GaugeValue(float64Ptr(1.25)),
			want:    "1.250000",
			wantErr: false,
		},
		{
			name:    "counter",
			mtype:   metrics.Counter,
			key:     "PollCount",
			value:   metrics.CounterValue(int64Ptr(9)),
			want:    "9",
			wantErr: false,
		},
		{
			name:    "unsupported type",
			mtype:   "histogram",
			key:     "x",
			value:   int64Ptr(1),
			wantErr: true,
		},
		{
			name:    "invalid gauge",
			mtype:   metrics.Gauge,
			key:     "Alloc",
			value:   int64Ptr(1),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatMetricValue(tt.mtype, tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("formatMetricValue() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("formatMetricValue() got = %q, want %q", got, tt.want)
			}
		})
	}
}

func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }
