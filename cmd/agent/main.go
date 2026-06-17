package main

import (
	"context"
	"errors"
	"flag"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/shigabutdinoff/metrics/internal/agent"
	config "github.com/shigabutdinoff/metrics/internal/config/agent"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

var (
	address           = flag.String("address", string(config.DefaultAddress), "HTTP server endpoint address")
	reportIntervalSec = flag.Int64("report-interval", int64(config.DefaultReportInterval), "report interval in seconds")
	pollIntervalSec   = flag.Int64("poll-interval", int64(config.DefaultPollInterval), "poll interval in seconds")
	key               = flag.String("key", "", "Секретный ключ для подписи")
	rateLimit         = flag.Int64("rate-limit", int64(config.DefaultRateLimit), "max concurrent outgoing requests")
)

func init() {
	flag.StringVar(address, "a", string(config.DefaultAddress), "HTTP server endpoint address (shorthand)")
	flag.Int64Var(reportIntervalSec, "r", int64(config.DefaultReportInterval), "report interval in seconds (shorthand)")
	flag.Int64Var(pollIntervalSec, "p", int64(config.DefaultPollInterval), "poll interval in seconds (shorthand)")
	flag.StringVar(key, "k", "", "Секретный ключ для подписи (shorthand)")
	flag.Int64Var(rateLimit, "l", int64(config.DefaultRateLimit), "max concurrent outgoing requests (shorthand)")
}

func main() {
	flag.Parse()

	// создаём предустановленный регистратор zap
	logger, err := zap.NewDevelopment()
	if err != nil {
		// вызываем панику, если ошибка
		panic(err)
	}
	defer logger.Sync()

	st := storage.NewMemStorage()
	a := agent.New(st, logger)

	if err = env.Parse(&a); err != nil {
		logger.Error("Не удалось распарить окружение", zap.Error(err))
	}

	a.Address = config.Address(normalizeAddress(string(a.Address)))
	a.PollInterval = time.Duration(a.PollIntervalInt64) * time.Second
	a.ReportInterval = time.Duration(a.ReportIntervalInt64) * time.Second

	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "address", "a":
			a.Address = config.Address(normalizeAddress(*address))
		case "report-interval", "r":
			a.ReportInterval = time.Duration(*reportIntervalSec) * time.Second
		case "poll-interval", "p":
			a.PollInterval = time.Duration(*pollIntervalSec) * time.Second
		case "key", "k":
			a.Key = *key
		case "rate-limit", "l":
			a.RateLimitInt64 = config.RateLimit(*rateLimit)
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err = a.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("Агент остановлен с ошибкой", zap.Error(err))
	}
}

func normalizeAddress(address string) string {
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address
	}

	return "http://" + address
}
