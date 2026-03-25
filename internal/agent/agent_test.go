package agent

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
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

	var methods []string
	var contentTypes []string
	var contentEncodings []string
	var acceptedEncodings []string
	var received []metrics.Metrics
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		contentTypes = append(contentTypes, r.Header.Get("Content-Type"))
		contentEncodings = append(contentEncodings, r.Header.Get("Content-Encoding"))
		acceptedEncodings = append(acceptedEncodings, r.Header.Get("Accept-Encoding"))

		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatalf("gzip.NewReader() error = %v", err)
		}
		defer zr.Close()

		body, err := io.ReadAll(zr)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}

		var metric metrics.Metrics
		if err := json.Unmarshal(body, &metric); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		received = append(received, metric)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a := &Agent{
		Storage: st,
		Client:  resty.NewWithClient(ts.Client()),
		Address: Address(ts.URL),
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
		if ct != "application/json" {
			t.Fatalf("content type = %q, want application/json", ct)
		}
	}
	for _, ce := range contentEncodings {
		if ce != "gzip" {
			t.Fatalf("content encoding = %q, want gzip", ce)
		}
	}
	for _, ae := range acceptedEncodings {
		if ae != "gzip" {
			t.Fatalf("accept encoding = %q, want gzip", ae)
		}
	}

	gotGauge := false
	gotCounter := false
	for _, metric := range received {
		switch metric.ID {
		case "Alloc":
			gotGauge = metric.MType == metrics.Gauge && metric.Value != nil && *metric.Value == 12.5
		case "PollCount":
			gotCounter = metric.MType == metrics.Counter && metric.Delta != nil && *metric.Delta == 7
		}
	}
	if !gotGauge {
		t.Fatal("gauge payload not received")
	}
	if !gotCounter {
		t.Fatal("counter payload not received")
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
				if r.URL.Path != "/update/" {
					t.Fatalf("path = %q, want /update/", r.URL.Path)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("content type = %q, want application/json", r.Header.Get("Content-Type"))
				}
				if r.Header.Get("Content-Encoding") != "gzip" {
					t.Fatalf("content encoding = %q, want gzip", r.Header.Get("Content-Encoding"))
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer ts.Close()

			a := &Agent{
				Client:  resty.NewWithClient(ts.Client()),
				Address: Address(ts.URL),
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
