package auditmw

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/audit"
)

type extractor func(r *http.Request, recorded []string) ([]string, error)

// FromPath аудирует запись метрики с именем в URL-параметре {name}.
func FromPath(n audit.Notifier, log *zap.Logger) func(http.Handler) http.Handler {
	return wrap(n, log, false, namesFromPath)
}

// FromBody аудирует запись метрик, имена хендлер передаёт через audit.Record.
func FromBody(n audit.Notifier, log *zap.Logger) func(http.Handler) http.Handler {
	return wrap(n, log, true, namesFromRecord)
}

func wrap(n audit.Notifier, log *zap.Logger, record bool, ex extractor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var recorded audit.Names
			if record {
				r = r.WithContext(audit.WithRecord(r.Context(), &recorded))
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			if status := ww.Status(); status < 200 || status > 299 {
				return
			}

			names, err := ex(r, recorded.Collected())
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

func namesFromRecord(_ *http.Request, recorded []string) ([]string, error) {
	if len(recorded) == 0 {
		return nil, errors.New("хендлер не передал имена метрик")
	}
	return recorded, nil
}

func namesFromPath(r *http.Request, _ []string) ([]string, error) {
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
