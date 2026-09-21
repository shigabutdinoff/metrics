package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/server"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/shigabutdinoff/metrics/pkg/buildinfo"
	"github.com/shigabutdinoff/metrics/pkg/jsonconfig"
)

var (
	address         = flag.String("address", server.DefaultAddress, "Адрес, на котором сервер слушает HTTP")
	storeInterval   = flag.Int("store-interval", server.DefaultStoreInterval, "Интервал времени в секундах")
	fileStoragePath = flag.String("file-storage-path", server.DefaultFileStoragePath, "Путь до файла")
	restore         = flag.Bool("restore", server.DefaultRestore, "Загружать ранее сохранённые значения")
	databaseDsn     = flag.String("database-dsn", server.DefaultDatabaseDSN, "Адрес подключения к БД")
	key             = flag.String("key", server.DefaultKey, "Секретный ключ для подписи")
	cryptoKey       = flag.String("crypto-key", server.DefaultCryptoKey, "Путь к файлу с приватным ключом")
	auditFile       = flag.String("audit-file", server.DefaultAuditFile, "Путь к файлу логов аудита")
	auditURL        = flag.String("audit-url", server.DefaultAuditURL, "URL приёмника логов аудита")
	pprofAddress    = flag.String("pprof-address", server.DefaultPprofAddress, "Адрес pprof, пусто выключает")
	trustedSubnet   = flag.String("trusted-subnet", server.DefaultTrustedSubnet, "Доверенная подсеть агентов в CIDR")
	grpcAddress     = flag.String("grpc-address", server.DefaultGRPCAddress, "Адрес gRPC, пусто выключает")
	configFile      = flag.String("config", "", "Путь к JSON-файлу конфигурации")
)

var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

func init() {
	flag.StringVar(address, "a", server.DefaultAddress, "Адрес, на котором сервер слушает HTTP (shorthand)")
	flag.IntVar(storeInterval, "i", server.DefaultStoreInterval, "Интервал времени в секундах (shorthand)")
	flag.StringVar(fileStoragePath, "f", server.DefaultFileStoragePath, "Путь до файла (shorthand)")
	flag.BoolVar(restore, "r", server.DefaultRestore, "Загружать ранее сохранённые значения (shorthand)")
	flag.StringVar(databaseDsn, "d", server.DefaultDatabaseDSN, "Адрес подключения к БД (shorthand)")
	flag.StringVar(key, "k", server.DefaultKey, "Секретный ключ для подписи (shorthand)")
	flag.StringVar(cryptoKey, "ck", server.DefaultCryptoKey, "Путь к файлу с приватным ключом (shorthand)")
	flag.StringVar(auditFile, "af", server.DefaultAuditFile, "Путь к файлу логов аудита (shorthand)")
	flag.StringVar(auditURL, "au", server.DefaultAuditURL, "URL приёмника логов аудита (shorthand)")
	flag.StringVar(pprofAddress, "pa", server.DefaultPprofAddress, "Адрес pprof, пусто выключает (shorthand)")
	flag.StringVar(trustedSubnet, "t", server.DefaultTrustedSubnet, "Доверенная подсеть агентов в CIDR (shorthand)")
	flag.StringVar(grpcAddress, "ga", server.DefaultGRPCAddress, "Адрес gRPC, пусто выключает (shorthand)")
	flag.StringVar(configFile, "c", "", "Путь к JSON-файлу конфигурации (shorthand)")
}

func main() {
	buildinfo.Print(os.Stdout, buildVersion, buildDate, buildCommit)

	if err := run(); err != nil {
		log.Fatalf("Сервер остановлен с ошибкой: %v", err)
	}
}

func run() error {
	flag.Parse()

	logger, err := zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("создание логгера: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	st := storage.NewMemStorage()
	s := server.New(st, logger)

	if path := cmp.Or(os.Getenv("CONFIG"), *configFile); path != "" {
		if err := jsonconfig.Load(path, s); err != nil {
			return fmt.Errorf("файл конфигурации %s: %w", path, err)
		}
	}

	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "address", "a":
			s.Address = *address
		case "store-interval", "i":
			s.StoreInterval = jsonconfig.Seconds(*storeInterval)
		case "file-storage-path", "f":
			s.FileStoragePath = *fileStoragePath
		case "restore", "r":
			s.Restore = *restore
		case "database-dsn", "d":
			s.DatabaseDSN = *databaseDsn
		case "key", "k":
			s.Key = *key
		case "crypto-key", "ck":
			s.CryptoKey = *cryptoKey
		case "audit-file", "af":
			s.AuditFile = *auditFile
		case "audit-url", "au":
			s.AuditURL = *auditURL
		case "pprof-address", "pa":
			s.PprofAddress = *pprofAddress
		case "trusted-subnet", "t":
			s.TrustedSubnet = *trustedSubnet
		case "grpc-address", "ga":
			s.GRPCAddress = *grpcAddress
		}
	})

	if err := env.Parse(s); err != nil {
		return fmt.Errorf("разбор окружения: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer stop()

	return s.Run(ctx)
}
