package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/agent"
	config "github.com/shigabutdinoff/metrics/internal/config/agent"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/shigabutdinoff/metrics/pkg/buildinfo"
	"github.com/shigabutdinoff/metrics/pkg/jsonconfig"
)

var (
	address           = flag.String("address", string(config.DefaultAddress), "Адрес сервера метрик")
	reportIntervalSec = flag.Int64("report-interval", int64(config.DefaultReportInterval), "Период отправки метрик в секундах")
	pollIntervalSec   = flag.Int64("poll-interval", int64(config.DefaultPollInterval), "Период снятия метрик в секундах")
	key               = flag.String("key", "", "Секретный ключ для подписи")
	cryptoKey         = flag.String("crypto-key", "", "Путь к файлу с публичным ключом")
	rateLimit         = flag.Int64("rate-limit", int64(config.DefaultRateLimit), "Число одновременных запросов к серверу")
	grpcAddress       = flag.String("grpc-address", "", "Адрес сервера gRPC, пусто шлёт по HTTP")
	configFile        = flag.String("config", "", "Путь к JSON-файлу конфигурации")
)

var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

func init() {
	flag.StringVar(address, "a", string(config.DefaultAddress), "Адрес сервера метрик (shorthand)")
	flag.Int64Var(reportIntervalSec, "r", int64(config.DefaultReportInterval), "Период отправки метрик в секундах (shorthand)")
	flag.Int64Var(pollIntervalSec, "p", int64(config.DefaultPollInterval), "Период снятия метрик в секундах (shorthand)")
	flag.StringVar(key, "k", "", "Секретный ключ для подписи (shorthand)")
	flag.StringVar(cryptoKey, "ck", "", "Путь к файлу с публичным ключом (shorthand)")
	flag.Int64Var(rateLimit, "l", int64(config.DefaultRateLimit), "Число одновременных запросов к серверу (shorthand)")
	flag.StringVar(grpcAddress, "ga", "", "Адрес сервера gRPC, пусто шлёт по HTTP (shorthand)")
	flag.StringVar(configFile, "c", "", "Путь к JSON-файлу конфигурации (shorthand)")
}

func main() {
	buildinfo.Print(os.Stdout, buildVersion, buildDate, buildCommit)

	flag.Parse()

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	st := storage.NewMemStorage()
	a := agent.New(st, logger)

	if path := cmp.Or(*configFile, os.Getenv("CONFIG")); path != "" {
		if err = jsonconfig.Load(path, &a.Config); err != nil {
			logger.Fatal("Не удалось прочитать файл конфигурации", zap.String("path", path), zap.Error(err))
		}
	}

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
		case "crypto-key", "ck":
			a.CryptoKey = *cryptoKey
		case "rate-limit", "l":
			a.RateLimitInt64 = config.RateLimit(*rateLimit)
		case "grpc-address", "ga":
			a.GRPCAddress = *grpcAddress
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer stop()

	if err = a.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatal("Агент остановлен с ошибкой", zap.Error(err))
	}
	logger.Info("Агент остановлен")
}

func normalizeAddress(address string) string {
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address
	}

	return "http://" + address
}
