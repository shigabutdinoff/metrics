package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/shigabutdinoff/metrics/internal/config/agent"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/repository"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

type Agent struct {
	Storage        storage.Storage
	Client         *resty.Client
	PollInterval   time.Duration
	ReportInterval time.Duration
	Logger         *zap.Logger
	agent.Config
}

func newClient() *resty.Client {
	c := resty.New().SetTimeout(10 * time.Second)
	c.SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second)
	c.AddRetryCondition(func(r *resty.Response, err error) bool {
		if err != nil {
			return true
		}
		if r == nil {
			return false
		}
		status := r.StatusCode()
		return status >= 500 && status <= 599
	})
	return c
}

func New(st storage.Storage, logger *zap.Logger) Agent {
	return Agent{
		Storage: st,
		Client:  newClient(),
		Config: agent.Config{
			PollIntervalInt64:   agent.DefaultPollInterval,
			ReportIntervalInt64: agent.DefaultReportInterval,
			Address:             agent.DefaultAddress,
		},
		Logger: logger,
	}
}

func (a *Agent) Run() {
	a.CollectMetrics()
	a.ReportMetrics()

	nextPoll := time.Now().Add(a.PollInterval)
	nextReport := time.Now().Add(a.ReportInterval)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		if !now.Before(nextPoll) {
			a.CollectMetrics()
			nextPoll = now.Add(a.PollInterval)
		}

		if !now.Before(nextReport) {
			a.ReportMetrics()
			nextReport = now.Add(a.ReportInterval)
		}
	}
}

func (a *Agent) CollectMetrics() {
	var m repository.MemStats
	runtime.ReadMemStats(&m.MemStats)

	ctx := context.Background()
	for name, value := range m.GetGauges() {
		v := value
		a.Storage.SetGauge(ctx, name, &v)
	}

	delta := int64(1)
	a.Storage.AddCounter(ctx, "PollCount", &delta)

	randomValue := rand.Float64()
	a.Storage.SetGauge(ctx, "RandomValue", &randomValue)
}

func (a *Agent) ReportMetrics() {
	ctx := context.Background()

	var batch []metrics.Metrics
	for name, value := range a.Storage.GetGauges(ctx) {
		if value == nil {
			continue
		}
		batch = append(batch, metrics.Metrics{ID: name, MType: metrics.Gauge, Value: value})
	}
	for name, value := range a.Storage.GetCounters(ctx) {
		if value == nil {
			continue
		}
		batch = append(batch, metrics.Metrics{ID: name, MType: metrics.Counter, Delta: value})
	}
	if len(batch) == 0 {
		return
	}
	if err := a.sendMetrics(ctx, batch); err != nil {
		a.Logger.Warn("Ошибка отправки метрик", zap.Error(err))
	}
}

func (a *Agent) sendMetrics(ctx context.Context, items []metrics.Metrics) error {
	body, err := json.Marshal(items)
	if err != nil {
		return err
	}

	var compressedBody bytes.Buffer
	zw := gzip.NewWriter(&compressedBody)
	if _, err := zw.Write(body); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	baseURL := a.Address
	path := fmt.Sprintf("%s/updates/", baseURL)

	resp, err := a.Client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Content-Encoding", "gzip").
		SetHeader("Accept-Encoding", "gzip").
		SetBody(compressedBody.Bytes()).
		Post(path)
	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("ошибка получения статуса %d", resp.StatusCode())
	}
	return nil
}

func formatMetricValue(mtype metrics.Type, name string, value any) (string, error) {
	switch mtype {
	case metrics.Gauge:
		gauge, ok := value.(metrics.GaugeValue)
		if !ok || gauge == nil {
			return "", fmt.Errorf("неверное gauge значение для %q", name)
		}
		return fmt.Sprintf("%f", *gauge), nil
	case metrics.Counter:
		counter, ok := value.(metrics.CounterValue)
		if !ok || counter == nil {
			return "", fmt.Errorf("неверное counter значение для %q", name)
		}
		return fmt.Sprintf("%d", *counter), nil
	default:
		return "", fmt.Errorf("неподдерживаемый тип метрики %s", mtype)
	}
}
