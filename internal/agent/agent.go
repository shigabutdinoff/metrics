package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"sync"
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
		return status == 429 || (status >= 500 && status <= 599)
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
			RateLimitInt64:      agent.DefaultRateLimit,
		},
		Logger: logger,
	}
}

// workerCount возвращает число воркеров пула отправки
func (a *Agent) workerCount() int {
	if n := int(a.Config.RateLimitInt64); n >= 1 {
		return n
	}
	return 1
}

// Run запускает агент
func (a *Agent) Run(ctx context.Context) error {
	workers := a.workerCount()
	jobs := make(chan metrics.Metrics, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range jobs {
				if err := a.sendMetrics(ctx, []metrics.Metrics{m}); err != nil {
					a.Logger.Warn("Ошибка отправки метрики", zap.String("id", m.ID), zap.Error(err))
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.collectLoop(ctx, a.PollInterval, a.CollectMetrics)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.collectLoop(ctx, a.PollInterval, func() { a.collectGopsutil(ctx) })
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.reportLoop(ctx, jobs)
	}()

	wg.Wait()
	return ctx.Err()
}

// collectLoop вызывает fn сразу и далее каждые d, пока не отменён ctx
func (a *Agent) collectLoop(ctx context.Context, d time.Duration, fn func()) {
	fn()

	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn()
		}
	}
}

// reportLoop каждые ReportInterval читает накопленные метрики и отправляет каждую отдельным заданием в пул
func (a *Agent) reportLoop(ctx context.Context, jobs chan<- metrics.Metrics) {
	defer close(jobs)

	ticker := time.NewTicker(a.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, m := range a.buildBatch(ctx) {
				select {
				case jobs <- m:
				case <-ctx.Done():
					return
				}
			}
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

// buildBatch собирает текущий снимок накопленных метрик из хранилища.
func (a *Agent) buildBatch(ctx context.Context) []metrics.Metrics {
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
	return batch
}

func (a *Agent) sendMetrics(ctx context.Context, items []metrics.Metrics) error {
	body, err := json.Marshal(items)
	if err != nil {
		return err
	}

	var hashHeader string
	if a.Config.Key != "" {
		mac := hmac.New(sha256.New, []byte(a.Config.Key))
		mac.Write(body)
		hashHeader = hex.EncodeToString(mac.Sum(nil))
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

	req := a.Client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Content-Encoding", "gzip").
		SetHeader("Accept-Encoding", "gzip").
		SetBody(compressedBody.Bytes())

	if hashHeader != "" {
		req.SetHeader("HashSHA256", hashHeader)
	}

	resp, err := req.Post(path)
	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("ошибка получения статуса %d", resp.StatusCode())
	}
	return nil
}
