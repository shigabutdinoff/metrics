package main

import (
	"github.com/Shigabutdinoff/metrics/internal/server"
	"github.com/Shigabutdinoff/metrics/internal/storage"
)

func main() {
	st := storage.NewMemStorage()
	server.New(st)
}
