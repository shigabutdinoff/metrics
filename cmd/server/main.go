package main

import (
	"flag"

	"github.com/caarlos0/env/v11"
	"github.com/shigabutdinoff/metrics/internal/server"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

var (
	address         = flag.String("address", server.DefaultAddress, "HTTP server endpoint address")
	storeInterval   = flag.Int("i", server.DefaultStoreInterval, "Интервал времени в секундах")
	fileStoragePath = flag.String("f", server.DefaultFileStoragePath, "Путь до файла")
	restore         = flag.Bool("r", server.DefaultRestore, "Загружать ранее сохранённые значения")
	databaseDsn     = flag.String("d", server.DefaultDatabaseDSN, "Адрес подключения к БД")
)

func init() {
	flag.StringVar(address, "a", server.DefaultAddress, "HTTP server endpoint address (shorthand)")
	flag.IntVar(storeInterval, "store-interval", server.DefaultStoreInterval, "Интервал времени в секундах")
	flag.StringVar(fileStoragePath, "file-storage-path", server.DefaultFileStoragePath, "Путь до файла")
	flag.BoolVar(restore, "restore", server.DefaultRestore, "Загружать ранее сохранённые значения")
	flag.StringVar(databaseDsn, "database-dsn", server.DefaultDatabaseDSN, "Адрес подключения к БД")
}

func main() {
	flag.Parse()

	// создаём предустановленный регистратор zap
	logger, err := zap.NewDevelopment()
	if err != nil {
		// вызываем панику, если ошибка
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	st := storage.NewMemStorage()
	s := server.New(st, logger)
	s.Address = *address
	s.StoreInterval = *storeInterval
	s.FileStoragePath = *fileStoragePath
	s.Restore = *restore
	s.DatabaseDSN = *databaseDsn

	err = env.Parse(s)
	if err != nil {
		logger.Error("Не удалось распарить окружение", zap.Error(err))
	}

	s.Run()
}
