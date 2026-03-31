package agent

type (
	PollInterval   int64
	ReportInterval int64
	Address        string
)

const (
	DefaultPollInterval   PollInterval   = 2
	DefaultReportInterval ReportInterval = 10
	DefaultAddress        Address        = "http://localhost:8080"
)

type Config struct {
	PollIntervalInt64   PollInterval   `env:"POLL_INTERVAL"`
	ReportIntervalInt64 ReportInterval `env:"REPORT_INTERVAL"`
	Address             Address        `env:"ADDRESS"`
}
