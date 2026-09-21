package agent

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/shigabutdinoff/metrics/internal/config/agent"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/repository"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/shigabutdinoff/metrics/pkg/rsacrypt"
)

// Agent собирает метрики и отправляет их на сервер.
type Agent struct {
	// Storage хранилище собранных метрик.
	Storage storage.Storage
	// Client HTTP-клиент с настроенными повторами.
	Client *resty.Client
	// PollInterval период снятия метрик из Config.PollIntervalInt64.
	PollInterval time.Duration
	// ReportInterval период отправки из Config.ReportIntervalInt64.
	ReportInterval time.Duration
	// Logger журнал, куда пишутся ошибки сбора и отправки.
	Logger *zap.Logger
	// PublicKey ключ шифрования тела запросов, Run читает его из Config.CryptoKey.
	PublicKey       *rsa.PublicKey
	shutdownTimeout time.Duration
	// Config конфигурация агента: адрес сервера, интервалы, ключ подписи.
	agent.Config
}

const defaultShutdownTimeout = 10 * time.Second

var gzipWriters = sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}

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

// New создаёт агент с настройками по умолчанию и клиентом с ретраями.
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

func (a *Agent) workerCount() int {
	if n := int(a.Config.RateLimitInt64); n >= 1 {
		return n
	}
	return 1
}

// Run собирает и шлёт метрики до отмены ctx, затем досылает очередь
// не дольше shutdownTimeout.
func (a *Agent) Run(ctx context.Context) error {
	if a.CryptoKey != "" {
		pub, err := rsacrypt.LoadPublicKey(a.CryptoKey)
		if err != nil {
			return fmt.Errorf("загрузка публичного ключа: %w", err)
		}
		a.PublicKey = pub
	}

	workers := a.workerCount()
	jobs := make(chan []metrics.Metrics, workers)

	g, gctx := errgroup.WithContext(ctx)
	sendCtx, cancelSend := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelSend()
	timeout := cmp.Or(a.shutdownTimeout, defaultShutdownTimeout)
	context.AfterFunc(ctx, func() {
		select {
		case <-time.After(timeout):
			a.Logger.Warn("Очередь метрик не отправлена за отведённое время",
				zap.Duration("timeout", timeout))
			cancelSend()
		case <-sendCtx.Done():
		}
	})

	for i := 0; i < workers; i++ {
		g.Go(func() error {
			for batch := range jobs {
				if err := a.sendMetrics(sendCtx, batch); err != nil {
					a.Logger.Warn("Ошибка отправки метрик", zap.Error(err))
				}
			}
			return nil
		})
	}

	g.Go(func() error {
		a.collectLoop(gctx, a.PollInterval, a.CollectMetrics)
		return nil
	})

	g.Go(func() error {
		a.collectLoop(gctx, a.PollInterval, func() { a.collectGopsutil(gctx) })
		return nil
	})

	g.Go(func() error {
		a.reportLoop(gctx, jobs)
		return nil
	})

	return g.Wait()
}

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

func (a *Agent) reportLoop(ctx context.Context, jobs chan<- []metrics.Metrics) {
	defer close(jobs)

	ticker := time.NewTicker(a.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			batch := a.buildBatch(ctx)
			if len(batch) == 0 {
				continue
			}
			// Собранная пачка уже в обработке, воркеры читают jobs до close.
			jobs <- batch
		}
	}
}

// CollectMetrics снимает метрики runtime, растит PollCount и RandomValue.
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

func (a *Agent) buildBatch(ctx context.Context) []metrics.Metrics {
	gauges := a.Storage.GetGauges(ctx)
	counters := a.Storage.GetCounters(ctx)
	batch := make([]metrics.Metrics, 0, len(gauges)+len(counters))
	for name, value := range gauges {
		if value == nil {
			continue
		}
		batch = append(batch, metrics.Metrics{ID: name, MType: metrics.Gauge, Value: value})
	}
	for name, value := range counters {
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
	zw := gzipWriters.Get().(*gzip.Writer)
	zw.Reset(&compressedBody)
	defer func() {
		zw.Reset(io.Discard)
		gzipWriters.Put(zw)
	}()
	if _, err := zw.Write(body); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	payload := compressedBody.Bytes()
	if a.PublicKey != nil {
		if payload, err = rsacrypt.Encrypt(a.PublicKey, payload); err != nil {
			return err
		}
	}

	path := string(a.Address) + "/updates/"

	req := a.Client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Content-Encoding", "gzip").
		SetHeader("Accept-Encoding", "gzip").
		SetBody(payload)

	if hashHeader != "" {
		req.SetHeader("HashSHA256", hashHeader)
	}
	if ip := a.realIP(ctx); ip != "" {
		req.SetHeader("X-Real-IP", ip)
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

func (a *Agent) realIP(ctx context.Context) string {
	ip, err := hostIP(ctx, string(a.Address))
	if err != nil {
		a.Logger.Warn("IP хоста не определён, X-Real-IP не отправляется", zap.Error(err))
		return ""
	}
	return ip.String()
}

func hostIP(ctx context.Context, address string) (net.IP, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	host := net.JoinHostPort(u.Hostname(), cmp.Or(u.Port(), "80"))
	conn, err := d.DialContext(ctx, "udp4", host)
	if err != nil {
		conn, err = d.DialContext(ctx, "udp", host)
	}
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}
