package value

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

func ShowTextPlain(st storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		name, err := url.PathUnescape(chi.URLParam(req, "name"))
		if err != nil || strings.TrimSpace(name) == "" {
			http.Error(res, fmt.Sprintf("invalid metric name: %v", err), http.StatusNotFound)
			return
		}

		mType := metrics.Type(chi.URLParam(req, "type"))
		switch mType {
		case metrics.Gauge:
			value := st.GetGauges()[name]
			if value == nil {
				http.Error(res, "metric not found", http.StatusNotFound)
				return
			}
			res.WriteHeader(http.StatusOK)
			_, _ = res.Write([]byte(strconv.FormatFloat(*value, 'f', -1, 64)))
		case metrics.Counter:
			value := st.GetCounters()[name]
			if value == nil {
				http.Error(res, "metric not found", http.StatusNotFound)
				return
			}
			res.WriteHeader(http.StatusOK)
			_, _ = res.Write([]byte(strconv.FormatInt(*value, 10)))
		default:
			http.Error(res, "invalid metric type", http.StatusBadRequest)
		}
	}
}

func ShowApplicationJson(st storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var upd metrics.Metrics
		if err := json.NewDecoder(req.Body).Decode(&upd); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(upd.ID) == "" {
			http.Error(res, "invalid metric name", http.StatusNotFound)
			return
		}

		switch upd.MType {
		case metrics.Gauge:
			value := st.GetGauges()[upd.ID]
			if value == nil {
				http.Error(res, "metric not found", http.StatusNotFound)
				return
			}
			upd.Value = value
			upd.Delta = nil
		case metrics.Counter:
			value := st.GetCounters()[upd.ID]
			if value == nil {
				http.Error(res, "metric not found", http.StatusNotFound)
				return
			}
			upd.Delta = value
			upd.Value = nil
		default:
			http.Error(res, "invalid metric type", http.StatusBadRequest)
			return
		}

		res.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(res).Encode(upd); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
