package main

import (
	"flag"
	"strings"
	"time"

	"github.com/Shigabutdinoff/metrics/internal/agent"
	"github.com/Shigabutdinoff/metrics/internal/storage"
)

var (
	serverAddress     = flag.String("address", "localhost:8080", "HTTP server endpoint address")
	reportIntervalSec = flag.Int("report-interval", int(agent.DefaultReportInterval/time.Second), "report interval in seconds")
	pollIntervalSec   = flag.Int("poll-interval", int(agent.DefaultPollInterval/time.Second), "poll interval in seconds")
)

func init() {
	flag.StringVar(serverAddress, "a", "localhost:8080", "HTTP server endpoint address (shorthand)")
	flag.IntVar(reportIntervalSec, "r", int(agent.DefaultReportInterval/time.Second), "report interval in seconds (shorthand)")
	flag.IntVar(pollIntervalSec, "p", int(agent.DefaultPollInterval/time.Second), "poll interval in seconds (shorthand)")
}

func main() {
	flag.Parse()

	st := storage.NewMemStorage()
	a := agent.New(st)
	a.ServerAddress = normalizeServerAddress(*serverAddress)
	a.ReportInterval = time.Duration(*reportIntervalSec) * time.Second
	a.PollInterval = time.Duration(*pollIntervalSec) * time.Second
	a.Run()
}

func normalizeServerAddress(address string) string {
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address
	}

	return "http://" + address
}
