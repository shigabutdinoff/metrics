package main

import (
	"flag"

	"github.com/caarlos0/env/v11"
	"github.com/shigabutdinoff/metrics/internal/server"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

var (
	address = flag.String("address", server.DefaultAddress, "HTTP server endpoint address")
)

func init() {
	flag.StringVar(address, "a", server.DefaultAddress, "HTTP server endpoint address (shorthand)")
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
	s := server.New(st, logger)
	s.Address = *address

	err = env.Parse(&s)
	if err != nil {
		logger.Error("Не удалось распарить окружение", zap.Error(err))
	}

	s.Run()
}
