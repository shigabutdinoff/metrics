package audit

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	requestTimeout = 1 * time.Second
	retryCount     = 2
	retryWaitTime  = 250 * time.Millisecond
	retryMaxWait   = 500 * time.Millisecond
)

// HTTPSink отправляет события аудита POST-запросом на удалённый сервер.
type HTTPSink struct {
	client *resty.Client
	url    string
}

// NewHTTPSink создаёт приёмник, проверяя, что URL пригоден для POST-запроса.
func NewHTTPSink(rawURL string) (*HTTPSink, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("некорректный URL приёмника аудита %q: ожидается http(s) с указанием хоста", rawURL)
	}

	client := resty.New().
		SetTimeout(requestTimeout).
		SetRetryCount(retryCount).
		SetRetryWaitTime(retryWaitTime).
		SetRetryMaxWaitTime(retryMaxWait).
		AddRetryCondition(retryCondition)

	return &HTTPSink{client: client, url: rawURL}, nil
}

func retryCondition(r *resty.Response, err error) bool {
	if err != nil {
		return true
	}
	if r == nil {
		return false
	}
	status := r.StatusCode()
	return status == http.StatusTooManyRequests || (status >= 500 && status <= 599)
}

// Update отправляет событие POST-запросом на настроенный URL.
func (s *HTTPSink) Update(ctx context.Context, e Event) error {
	resp, err := s.client.R().
		SetContext(ctx).
		SetBody(e).
		Post(s.url)
	if err != nil {
		return err
	}

	if !resp.IsSuccess() {
		return fmt.Errorf("ошибка получения статуса %d", resp.StatusCode())
	}

	return nil
}
