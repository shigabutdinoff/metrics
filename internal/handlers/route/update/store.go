package update

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/reqbody"
	"github.com/shigabutdinoff/metrics/internal/handlers/request"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

// StoreTextPlain обрабатывает POST /update/{type}/{name}/{value}.
func StoreTextPlain(st storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		upd := &request.Update{Request: req}
		code, err := upd.Validate()
		if err != nil {
			http.Error(res, err.Error(), code)
			return
		}

		switch upd.MType {
		case metrics.Gauge:
			st.SetGauge(req.Context(), upd.ID, upd.Value)
		case metrics.Counter:
			st.AddCounter(req.Context(), upd.ID, upd.Delta)
		default:
			http.Error(res, "неверный тип метрики", http.StatusBadRequest)
			return
		}

		res.WriteHeader(code)
		_, _ = res.Write([]byte("OK"))
	}
}

// StoreApplicationJSON обрабатывает POST /update/ и отдаёт присланное.
func StoreApplicationJSON(st storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var upd metrics.Metrics
		if err := json.NewDecoder(req.Body).Decode(&upd); err != nil {
			reqbody.Error(res, err)
			return
		}

		if strings.TrimSpace(upd.ID) == "" {
			http.Error(res, "неверное название метрики", http.StatusNotFound)
			return
		}

		switch upd.MType {
		case metrics.Gauge:
			st.SetGauge(req.Context(), upd.ID, upd.Value)
		case metrics.Counter:
			st.AddCounter(req.Context(), upd.ID, upd.Delta)
		default:
			http.Error(res, "неверный тип метрики", http.StatusBadRequest)
			return
		}
		audit.Record(req.Context(), upd.ID)

		res.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(res).Encode(upd); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
