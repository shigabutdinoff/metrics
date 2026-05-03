package updates

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

func StoreApplicationJSONBatch(st storage.Storage, logger *zap.Logger) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var items []metrics.Metrics

		if err := json.NewDecoder(req.Body).Decode(&items); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		if len(items) == 0 {
			http.Error(res, "Отсутствует тело запроса", http.StatusBadRequest)
			return
		}

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
