package update

import (
	"net/http"

	"github.com/shigabutdinoff/metrics/internal/handlers/request"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

func Store(st storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		upd := &request.Update{Request: req}
		code, err := upd.Validate()
		if err != nil {
			http.Error(res, err.Error(), code)
			return
		}

		switch upd.MType {
		case metrics.Gauge:
			st.SetGauge(upd.ID, upd.Value)
		case metrics.Counter:
			st.AddCounter(upd.ID, upd.Delta)
		default:
			http.Error(res, "invalid metric type", http.StatusBadRequest)
			return
		}

		res.WriteHeader(code)
		_, _ = res.Write([]byte("OK"))
	}
}
