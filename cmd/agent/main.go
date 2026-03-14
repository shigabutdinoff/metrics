package main

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/shigabutdinoff/metrics/internal/agent"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

var (
	address           = flag.String("address", string(agent.DefaultAddress), "HTTP server endpoint address")
	reportIntervalSec = flag.Int64("report-interval", int64(agent.DefaultReportInterval), "report interval in seconds")
	pollIntervalSec   = flag.Int64("poll-interval", int64(agent.DefaultPollInterval), "poll interval in seconds")
)

func init() {
	flag.StringVar(address, "a", string(agent.DefaultAddress), "HTTP server endpoint address (shorthand)")
	flag.Int64Var(reportIntervalSec, "r", int64(agent.DefaultReportInterval), "report interval in seconds (shorthand)")
	flag.Int64Var(pollIntervalSec, "p", int64(agent.DefaultPollInterval), "poll interval in seconds (shorthand)")
}

func main() {
	flag.Parse()

	st := storage.NewMemStorage()
	a := agent.New(st)
	a.Address = agent.Address(*address)
	a.ReportInterval = time.Duration(*reportIntervalSec)
	a.PollInterval = time.Duration(*pollIntervalSec)

	err := env.Parse(&a)
	a.Address = agent.Address(normalizeAddress(string(a.Address)))
	a.PollInterval = time.Duration(a.PollIntervalInt64) * time.Second
	a.ReportInterval = time.Duration(a.ReportIntervalInt64) * time.Second
	if err != nil {
		log.Fatal(err)
	}

	a.Run()
}

func normalizeAddress(address string) string {
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address
	}

	return "http://" + address
}
