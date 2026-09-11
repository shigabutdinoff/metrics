package updates

import (
	"encoding/json"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/reqbody"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

// StoreApplicationJSONBatch обрабатывает POST /updates/ с массивом метрик.
func StoreApplicationJSONBatch(st storage.Storage, logger *zap.Logger) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var items []metrics.Metrics

		if err := json.NewDecoder(req.Body).Decode(&items); err != nil {
			reqbody.Error(res, err)
			return
		}

		if len(items) == 0 {
			http.Error(res, "Отсутствует тело запроса", http.StatusBadRequest)
			return
		}

		names := make([]string, 0, len(items))
		for _, it := range items {
			if strings.TrimSpace(it.ID) == "" {
				http.Error(res, "Неверное название метрики", http.StatusNotFound)
				return
			}

			switch it.MType {
			case metrics.Gauge:
				st.SetGauge(req.Context(), it.ID, it.Value)
			case metrics.Counter:
				st.AddCounter(req.Context(), it.ID, it.Delta)
			default:
				http.Error(res, "Неверный тип метрики", http.StatusBadRequest)
				return
			}
			names = append(names, it.ID)
		}
		audit.Record(req.Context(), names...)

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
