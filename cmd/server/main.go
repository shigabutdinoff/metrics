package main

import (
	"flag"
	"log"

	"github.com/caarlos0/env/v11"
	"github.com/shigabutdinoff/metrics/internal/server"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

var (
	address = flag.String("address", server.DefaultAddress, "HTTP server endpoint address")
)

func init() {
	flag.StringVar(address, "a", server.DefaultAddress, "HTTP server endpoint address (shorthand)")
}

func main() {
	flag.Parse()

	st := storage.NewMemStorage()
	s := server.New(st)
	s.Address = *address

	err := env.Parse(&s)
	if err != nil {
		log.Fatal(err)
	}

	s.Run()
}
