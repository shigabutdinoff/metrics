package auditmw

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"go.uber.org/zap"
)

const maxBodySize = 10 << 20

type extractor func(r *http.Request, body []byte) ([]string, error)

// FromPath аудирует запись метрики с именем в URL-параметре {name}.
func FromPath(n audit.Notifier, log *zap.Logger) func(http.Handler) http.Handler {
	return wrap(n, log, namesFromPath)
}

// FromBody аудирует запись метрик из JSON-тела: объект или массив.
func FromBody(n audit.Notifier, log *zap.Logger) func(http.Handler) http.Handler {
	return wrap(n, log, namesFromBody)
}

func wrap(n audit.Notifier, log *zap.Logger, ex extractor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, ok := readBody(w, r)
			if !ok {
				return
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			if status := ww.Status(); status < 200 || status > 299 {
				return
			}

			names, err := ex(r, body)
			if err != nil {
				log.Warn("Не удалось определить наименования метрик для аудита", zap.Error(err))
				return
			}

			n.Publish(audit.Event{
				TS:        time.Now().Unix(),
				Metrics:   names,
				IPAddress: clientIP(r),
			})
		})
	}
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, true
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "тело запроса слишком велико", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "не удалось прочитать тело запроса", http.StatusBadRequest)
		}
		return nil, false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	return body, true
}

func namesFromBody(_ *http.Request, body []byte) ([]string, error) {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []metrics.Metrics
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		names := make([]string, len(items))
		for i, it := range items {
			names[i] = it.ID
		}
		return names, nil
	}

	var m metrics.Metrics
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return nil, err
	}
	return []string{m.ID}, nil
}

func namesFromPath(r *http.Request, _ []byte) ([]string, error) {
	name, err := url.PathUnescape(chi.URLParam(r, "name"))
	if err != nil {
		return nil, err
	}
	return []string{name}, nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
