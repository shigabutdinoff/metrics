package updates

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/reqbody"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/service/batch"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/shigabutdinoff/metrics/pkg/hostaddr"
)

// StoreApplicationJSONBatch обрабатывает POST /updates/ с массивом метрик.
// Событие аудита уходит в n, nil отключает аудит.
func StoreApplicationJSONBatch(
	st storage.Storage,
	n audit.Notifier,
	logger *zap.Logger,
) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var items []metrics.Metrics

		if err := json.NewDecoder(req.Body).Decode(&items); err != nil {
			reqbody.Error(res, err)
			return
		}

		if err := batch.Update(req.Context(), st, n, hostaddr.Host(req.RemoteAddr), items); err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, batch.ErrName) {
				code = http.StatusNotFound
			}
			http.Error(res, err.Error(), code)
			return
		}

		res.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(res).Encode(items); err != nil {
			if logger != nil {
				logger.Error("произошла ошибка шифрования", zap.Error(err))
			}
			http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
}
