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

func New(st storage.Storage, logger *zap.Logger) Agent {
	return Agent{
		Storage: st,
		Client:  resty.New().SetTimeout(10 * time.Second),
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
	if !a.UseBatch {
		for name, value := range a.Storage.GetGauges(ctx) {
			if value == nil {
				continue
			}
			if err := a.sendMetric(metrics.Gauge, name, value); err != nil {
				a.Logger.Warn("Ошибка отправки gauge "+name, zap.Error(err))
			}
		}
		for name, value := range a.Storage.GetCounters(ctx) {
			if value == nil {
				continue
			}
			if err := a.sendMetric(metrics.Counter, name, value); err != nil {
				a.Logger.Warn("Ошибка отправки counter "+name, zap.Error(err))
			}
		}
		return
	}

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
	if err := a.sendBatch(batch); err != nil {
		for _, m := range batch {
			var val any
			switch m.MType {
			case metrics.Gauge:
				val = m.Value
			case metrics.Counter:
				val = m.Delta
			default:
				continue
			}
			if err := a.sendMetric(m.MType, m.ID, val); err != nil {
				a.Logger.Warn("Ошибка отправки метрик "+m.ID, zap.Error(err))
			}
		}
	}
}

func (a *Agent) sendMetric(mtype metrics.Type, name string, value any) error {
	requestBody := metrics.Metrics{
		ID:    name,
		MType: mtype,
	}
	switch mtype {
	case metrics.Gauge:
		gauge, ok := value.(metrics.GaugeValue)
		if !ok || gauge == nil {
			return fmt.Errorf("неверное gauge значение для %q", name)
		}
		requestBody.Value = gauge
	case metrics.Counter:
		counter, ok := value.(metrics.CounterValue)
		if !ok || counter == nil {
			return fmt.Errorf("неверное counter значение для %q", name)
		}
		requestBody.Delta = counter
	default:
		return fmt.Errorf("неподдерживаемый тип метрики %s", mtype)
	}

	body, err := json.Marshal(requestBody)
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
	path := fmt.Sprintf("%s/update/", baseURL)

	resp, err := a.Client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Content-Encoding", "gzip").
		SetHeader("Accept-Encoding", "gzip").
		SetBody(compressedBody.Bytes()).
		Post(path)
	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("ошибка получения статуса  %d", resp.StatusCode())
	}

	return nil
}

func (a *Agent) sendBatch(items []metrics.Metrics) error {
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
