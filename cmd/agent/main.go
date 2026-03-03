package main

import (
	"github.com/Shigabutdinoff/metrics/internal/agent"
	"github.com/Shigabutdinoff/metrics/internal/storage"
)

func main() {
	st := storage.NewMemStorage()
	a := agent.New(st)
	a.Run()
}
