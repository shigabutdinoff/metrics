package main

import (
	"flag"

	"github.com/Shigabutdinoff/metrics/internal/server"
	"github.com/Shigabutdinoff/metrics/internal/storage"
)

var (
	addr = flag.String("address", "localhost:8080", "HTTP server endpoint address")
)

func init() {
	flag.StringVar(addr, "a", "localhost:8080", "HTTP server endpoint address (shorthand)")
}

func main() {
	flag.Parse()

	st := storage.NewMemStorage()
	server.New(st, *addr)
}
