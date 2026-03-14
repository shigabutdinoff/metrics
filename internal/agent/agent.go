package agent

import (
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/repository"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

type (
	PollInterval   int64
	ReportInterval int64
	Address        string
)

const (
	DefaultPollInterval   PollInterval   = 2
	DefaultReportInterval ReportInterval = 10
	DefaultAddress        Address        = "http://localhost:8080"
)

type Agent struct {
	Storage             storage.Storage
	Client              *resty.Client
	PollInterval        time.Duration
	ReportInterval      time.Duration
	PollIntervalInt64   PollInterval   `env:"POLL_INTERVAL"`
	ReportIntervalInt64 ReportInterval `env:"REPORT_INTERVAL"`
	Address             Address        `env:"ADDRESS"`
}

func New(st storage.Storage) Agent {
	return Agent{
		Storage:             st,
		Client:              resty.New(),
		PollIntervalInt64:   DefaultPollInterval,
		ReportIntervalInt64: DefaultReportInterval,
		Address:             DefaultAddress,
	}
}

func (a *Agent) Run() {
	a.CollectMetrics()
	a.ReportMetrics()

	nextPoll := time.Now().Add(a.PollInterval)
	nextReport := time.Now().Add(a.ReportInterval)

	for {
		now := time.Now()

		if !now.Before(nextPoll) {
			a.CollectMetrics()
			nextPoll = now.Add(a.PollInterval)
		}

		if !now.Before(nextReport) {
			a.ReportMetrics()
			nextReport = now.Add(a.ReportInterval)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (a *Agent) CollectMetrics() {
	var m repository.MemStats
	runtime.ReadMemStats(&m.MemStats)

	for name, value := range m.GetGauges() {
		v := value
		a.Storage.SetGauge(name, &v)
	}

	delta := int64(1)
	a.Storage.AddCounter("PollCount", &delta)

	randomValue := rand.Float64()
	a.Storage.SetGauge("RandomValue", &randomValue)
}

func (a *Agent) ReportMetrics() {
	for name, value := range a.Storage.GetGauges() {
		if value == nil {
			continue
		}
		if err := a.sendMetric(metrics.Gauge, name, value); err != nil {
			fmt.Printf("failed to send gauge %q: %v", name, err)
		}
	}

	for name, value := range a.Storage.GetCounters() {
		if value == nil {
			continue
		}
		if err := a.sendMetric(metrics.Counter, name, value); err != nil {
			fmt.Printf("failed to send counter %q: %v", name, err)
		}
	}
}

func (a *Agent) sendMetric(mtype metrics.Type, name string, value any) error {
	formattedValue, err := formatMetricValue(mtype, name, value)
	if err != nil {
		return err
	}

	baseURL := a.Address
	path := fmt.Sprintf("%s/update/%s/%s/%s", baseURL, mtype, name, formattedValue)
	resp, err := a.Client.R().
		SetHeader("Content-Type", "text/plain").
		Post(path)
	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode())
	}

	return nil
}

func formatMetricValue(mtype metrics.Type, name string, value any) (string, error) {
	switch mtype {
	case metrics.Gauge:
		gauge, ok := value.(metrics.GaugeValue)
		if !ok || gauge == nil {
			return "", fmt.Errorf("invalid gauge value for %q", name)
		}
		return fmt.Sprintf("%f", *gauge), nil
	case metrics.Counter:
		counter, ok := value.(metrics.CounterValue)
		if !ok || counter == nil {
			return "", fmt.Errorf("invalid counter value for %q", name)
		}
		return fmt.Sprintf("%d", *counter), nil
	default:
		return "", fmt.Errorf("unsupported metric type: %s", mtype)
	}
}
