package agent

type (
	// PollInterval период снятия метрик в секундах.
	PollInterval int64
	// ReportInterval период отправки метрик на сервер в секундах.
	ReportInterval int64
	// Address адрес сервера метрик вместе со схемой.
	Address string
	// RateLimit число одновременных запросов к серверу.
	RateLimit int64
)

// Значения конфигурации агента по умолчанию.
const (
	// DefaultPollInterval метрики снимаются раз в две секунды.
	DefaultPollInterval PollInterval = 2
	// DefaultReportInterval метрики отправляются раз в десять секунд.
	DefaultReportInterval ReportInterval = 10
	// DefaultAddress агент шлёт метрики на локальный порт 8080.
	DefaultAddress Address = "http://localhost:8080"
	// DefaultRateLimit запросы уходят по одному.
	DefaultRateLimit RateLimit = 1
)

// Config конфигурация агента, поля с тегом env читаются из окружения.
type Config struct {
	// PollIntervalInt64 период снятия метрик, флаг -p.
	PollIntervalInt64 PollInterval `env:"POLL_INTERVAL"`
	// ReportIntervalInt64 период отправки метрик, флаг -r.
	ReportIntervalInt64 ReportInterval `env:"REPORT_INTERVAL"`
	// Address адрес сервера метрик, флаг -a.
	Address Address `env:"ADDRESS"`
	// UseBatch отправлять ли метрики пачкой на /updates/ вместо /update/.
	UseBatch bool `env:"USE_BATCH"`
	// Key ключ подписи HMAC-SHA256, флаг -k.
	Key string `env:"KEY"`
	// CryptoKey путь к сертификату с публичным ключом, флаг -crypto-key.
	CryptoKey string `env:"CRYPTO_KEY"`
	// RateLimitInt64 число одновременных запросов к серверу, флаг -l.
	RateLimitInt64 RateLimit `env:"RATE_LIMIT"`
}
