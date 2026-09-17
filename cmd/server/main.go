package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/caarlos0/env/v11"
	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/server"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/shigabutdinoff/metrics/pkg/buildinfo"
)

var (
	address         = flag.String("address", server.DefaultAddress, "HTTP server endpoint address")
	storeInterval   = flag.Int("i", server.DefaultStoreInterval, "Интервал времени в секундах")
	fileStoragePath = flag.String("f", server.DefaultFileStoragePath, "Путь до файла")
	restore         = flag.Bool("r", server.DefaultRestore, "Загружать ранее сохранённые значения")
	databaseDsn     = flag.String("d", server.DefaultDatabaseDSN, "Адрес подключения к БД")
	key             = flag.String("k", server.DefaultKey, "Секретный ключ для подписи")
	cryptoKey       = flag.String("crypto-key", server.DefaultCryptoKey, "Путь к файлу с приватным ключом")
	auditFile       = flag.String("audit-file", server.DefaultAuditFile, "Путь к файлу логов аудита")
	auditURL        = flag.String("audit-url", server.DefaultAuditURL, "URL приёмника логов аудита")
	pprofAddress    = flag.String("pprof-address", server.DefaultPprofAddress, "Адрес pprof, пусто выключает")
)

var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

func init() {
	flag.StringVar(address, "a", server.DefaultAddress, "HTTP server endpoint address (shorthand)")
	flag.IntVar(storeInterval, "store-interval", server.DefaultStoreInterval, "Интервал времени в секундах")
	flag.StringVar(fileStoragePath, "file-storage-path", server.DefaultFileStoragePath, "Путь до файла")
	flag.BoolVar(restore, "restore", server.DefaultRestore, "Загружать ранее сохранённые значения")
	flag.StringVar(databaseDsn, "database-dsn", server.DefaultDatabaseDSN, "Адрес подключения к БД")
	flag.StringVar(key, "key", server.DefaultKey, "Секретный ключ для подписи")
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
	s.Address = *address
	s.StoreInterval = *storeInterval
	s.FileStoragePath = *fileStoragePath
	s.Restore = *restore
	s.DatabaseDSN = *databaseDsn
	s.Key = *key
	s.CryptoKey = *cryptoKey
	s.AuditFile = *auditFile
	s.AuditURL = *auditURL
	s.PprofAddress = *pprofAddress

	if err := env.Parse(s); err != nil {
		return fmt.Errorf("разбор окружения: %w", err)
	}

	return s.Run()
}
