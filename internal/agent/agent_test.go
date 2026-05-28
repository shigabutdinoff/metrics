package agent

import (
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
	config "github.com/shigabutdinoff/metrics/internal/config/agent"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

func TestAgent_CollectMetrics(t *testing.T) {
	st := storage.NewMemStorage()
	a := &Agent{Storage: st}

	a.CollectMetrics()

	poll := st.GetCounters(context.Background())["PollCount"]
	if poll == nil {
		t.Fatal("счётчик PollCount не установлен")
	}
	if *poll != 1 {
		t.Fatalf("PollCount = %d, ожидается 1", *poll)
	}

	randomValue := st.GetGauges(context.Background())["RandomValue"]
	if randomValue == nil {
		t.Fatal("gauge RandomValue не установлен")
	}

	alloc := st.GetGauges(context.Background())["Alloc"]
	if alloc == nil {
		t.Fatal("Alloc gauge из runtime-статистики не установлен")
	}
}

func TestAgent_ReportMetrics(t *testing.T) {
	st := storage.NewMemStorage()
	g := 12.5
	c := int64(7)
	st.SetGauge(context.Background(), "Alloc", &g)
	st.AddCounter(context.Background(), "PollCount", &c)

	var methods []string
	var contentTypes []string
	var contentEncodings []string
	var acceptedEncodings []string
	var received [][]metrics.Metrics
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

		if r.URL.Path != "/updates/" {
			t.Fatalf("путь = %q, ожидается /updates/", r.URL.Path)
		}

		var metricsBatch []metrics.Metrics
		if err := json.Unmarshal(body, &metricsBatch); err != nil {
			t.Fatalf("json.Unmarshal() ошибка = %v", err)
		}
		received = append(received, metricsBatch)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a := &Agent{
		Storage: st,
		Client:  resty.NewWithClient(ts.Client()),
		Config:  config.Config{Address: config.Address(ts.URL)},
	}

	a.ReportMetrics()

	if len(received) != 1 {
		t.Fatalf("количество запросов = %d, ожидается 1", len(received))
	}
	for _, method := range methods {
		if method != http.MethodPost {
			t.Fatalf("метод = %s, ожидается %s", method, http.MethodPost)
		}
	}
	for _, ct := range contentTypes {
		if ct != "application/json" {
			t.Fatalf("content type = %q, ожидается application/json", ct)
		}
	}
	for _, ce := range contentEncodings {
		if ce != "gzip" {
			t.Fatalf("content encoding = %q, ожидается gzip", ce)
		}
	}
	for _, ae := range acceptedEncodings {
		if ae != "gzip" {
			t.Fatalf("accept encoding = %q, ожидается gzip", ae)
		}
	}

	batch := received[0]
	if len(batch) != 2 {
		t.Fatalf("размер пакета = %d, ожидается 2", len(batch))
	}
	gotGauge := false
	gotCounter := false
	for _, metric := range batch {
		switch metric.ID {
		case "Alloc":
			gotGauge = metric.MType == metrics.Gauge && metric.Value != nil && *metric.Value == 12.5
		case "PollCount":
			gotCounter = metric.MType == metrics.Counter && metric.Delta != nil && *metric.Delta == 7
		}
	}
	if !gotGauge {
		t.Fatal("полезная нагрузка gauge не получена")
	}
	if !gotCounter {
		t.Fatal("полезная нагрузка counter не получена")
	}
}

func TestAgent_SendMetrics_Hash(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		wantHash bool
	}{
		{name: "ключ задан - заголовок HashSHA256 выставлен", key: "secret", wantHash: true},
		{name: "ключ пуст - заголовок HashSHA256 отсутствует", key: "", wantHash: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHeader string
			var hashMatched bool
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// агент шлёт gzip - распаковываем, чтобы проверить хэш несжатого тела
				zr, err := gzip.NewReader(r.Body)
				if err != nil {
					t.Fatalf("gzip.NewReader() ошибка = %v", err)
				}
				defer zr.Close()

				body, err := io.ReadAll(zr)
				if err != nil {
					t.Fatalf("io.ReadAll() ошибка = %v", err)
				}

				gotHeader = r.Header.Get("HashSHA256")
				if tt.key != "" {
					mac := hmac.New(sha256.New, []byte(tt.key))
					mac.Write(body)
					want := hex.EncodeToString(mac.Sum(nil))
					hashMatched = hmac.Equal([]byte(gotHeader), []byte(want))
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer ts.Close()

			g := 1.5
			a := &Agent{
				Storage: storage.NewMemStorage(),
				Client:  resty.NewWithClient(ts.Client()),
				Config:  config.Config{Address: config.Address(ts.URL), Key: tt.key},
			}

			items := []metrics.Metrics{{ID: "cpu", MType: metrics.Gauge, Value: &g}}
			if err := a.sendMetrics(context.Background(), items); err != nil {
				t.Fatalf("sendMetrics() ошибка = %v", err)
			}

			if tt.wantHash {
				if gotHeader == "" {
					t.Fatal("ожидался заголовок HashSHA256, но он отсутствует")
				}
				if !hashMatched {
					t.Fatalf("HashSHA256 не совпал с хэшем несжатого тела: %q", gotHeader)
				}
			} else if gotHeader != "" {
				t.Fatalf("заголовок HashSHA256 не ожидался, получен %q", gotHeader)
			}
		})
	}
}

func TestAgent_Run(t *testing.T) {
	t.Skip("Run содержит бесконечный цикл и требует отмены интеграционного теста")
}

func TestNew(t *testing.T) {
	st := storage.NewMemStorage()

	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Sync()

	got := New(st, logger)

	if got.Storage != st {
		t.Fatal("экземпляр хранилища не присвоен")
	}
	if got.Client == nil {
		t.Fatal("клиент равен nil")
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
			name:    "неподдерживаемый тип",
			mtype:   "histogram",
			key:     "x",
			value:   int64Ptr(1),
			wantErr: true,
		},
		{
			name:    "неверный gauge",
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
				t.Fatalf("formatMetricValue() ошибка = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("formatMetricValue() получено = %q, ожидается %q", got, tt.want)
			}
		})
	}
}

func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }
