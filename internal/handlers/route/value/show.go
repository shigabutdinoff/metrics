package value

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Shigabutdinoff/metrics/internal/model/metrics"
	"github.com/Shigabutdinoff/metrics/internal/storage"
	"github.com/go-chi/chi/v5"
)

func Show(st storage.Storage) http.HandlerFunc {
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
