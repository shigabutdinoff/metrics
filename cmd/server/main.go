// Команда server принимает и хранит метрики.
//
// Параметры задаются флагами и переменными окружения:
//
//	-a, -address            адрес, на котором сервер слушает HTTP
//	-i, -store-interval     период сохранения метрик в файл в секундах
//	-f, -file-storage-path  путь к файлу с метриками
//	-r, -restore            восстанавливать ли метрики из файла при старте
//	-d, -database-dsn       строка подключения к PostgreSQL
//	-k, -key                ключ подписи HMAC-SHA256
//	-audit-file             путь к файлу аудита
//	-audit-url              адрес приёмника аудита
//	-pprof-address          адрес сервера pprof, пустой отключает его
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/caarlos0/env/v11"
	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/server"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

var (
	address         = flag.String("address", server.DefaultAddress, "HTTP server endpoint address")
	storeInterval   = flag.Int("i", server.DefaultStoreInterval, "Интервал времени в секундах")
	fileStoragePath = flag.String("f", server.DefaultFileStoragePath, "Путь до файла")
	restore         = flag.Bool("r", server.DefaultRestore, "Загружать ранее сохранённые значения")
	databaseDsn     = flag.String("d", server.DefaultDatabaseDSN, "Адрес подключения к БД")
	key             = flag.String("k", server.DefaultKey, "Секретный ключ для подписи")
	auditFile       = flag.String("audit-file", server.DefaultAuditFile, "Путь к файлу логов аудита")
	auditURL        = flag.String("audit-url", server.DefaultAuditURL, "URL приёмника логов аудита")
	pprofAddress    = flag.String("pprof-address", server.DefaultPprofAddress, "Адрес pprof, пусто выключает")
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
	s.AuditFile = *auditFile
	s.AuditURL = *auditURL
	s.PprofAddress = *pprofAddress

	if err := env.Parse(s); err != nil {
		return fmt.Errorf("разбор окружения: %w", err)
	}

	return s.Run()
}
