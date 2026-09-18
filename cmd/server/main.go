package main

import (
	"cmp"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/caarlos0/env/v11"
	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/server"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/shigabutdinoff/metrics/pkg/buildinfo"
	"github.com/shigabutdinoff/metrics/pkg/jsonconfig"
)

var (
	address         = flag.String("address", server.DefaultAddress, "Адрес, на котором сервер слушает HTTP")
	storeInterval   = flag.Int("i", server.DefaultStoreInterval, "Интервал времени в секундах")
	fileStoragePath = flag.String("f", server.DefaultFileStoragePath, "Путь до файла")
	restore         = flag.Bool("r", server.DefaultRestore, "Загружать ранее сохранённые значения")
	databaseDsn     = flag.String("d", server.DefaultDatabaseDSN, "Адрес подключения к БД")
	key             = flag.String("k", server.DefaultKey, "Секретный ключ для подписи")
	cryptoKey       = flag.String("crypto-key", server.DefaultCryptoKey, "Путь к файлу с приватным ключом")
	auditFile       = flag.String("audit-file", server.DefaultAuditFile, "Путь к файлу логов аудита")
	auditURL        = flag.String("audit-url", server.DefaultAuditURL, "URL приёмника логов аудита")
	pprofAddress    = flag.String("pprof-address", server.DefaultPprofAddress, "Адрес pprof, пусто выключает")
	configFile      = flag.String("config", "", "Путь к JSON-файлу конфигурации")
)

var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

func init() {
	flag.StringVar(address, "a", server.DefaultAddress, "Адрес, на котором сервер слушает HTTP (shorthand)")
	flag.IntVar(storeInterval, "store-interval", server.DefaultStoreInterval, "Интервал времени в секундах")
	flag.StringVar(fileStoragePath, "file-storage-path", server.DefaultFileStoragePath, "Путь до файла")
	flag.BoolVar(restore, "restore", server.DefaultRestore, "Загружать ранее сохранённые значения")
	flag.StringVar(databaseDsn, "database-dsn", server.DefaultDatabaseDSN, "Адрес подключения к БД")
	flag.StringVar(key, "key", server.DefaultKey, "Секретный ключ для подписи")
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
		case "crypto-key":
			s.CryptoKey = *cryptoKey
		case "audit-file":
			s.AuditFile = *auditFile
		case "audit-url":
			s.AuditURL = *auditURL
		case "pprof-address":
			s.PprofAddress = *pprofAddress
		}
	})

	if err := env.Parse(s); err != nil {
		return fmt.Errorf("разбор окружения: %w", err)
	}

	return s.Run()
}
