package request

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Shigabutdinoff/metrics/internal/model/metrics"
	"github.com/go-chi/chi/v5"
)

type Update struct {
	*http.Request
	metrics.Metrics
}

func (u *Update) Validate() (int, error) {
	rawName := chi.URLParam(u.Request, "name")
	name, err := url.PathUnescape(rawName)
	if err != nil || strings.TrimSpace(name) == "" {
		return http.StatusNotFound, fmt.Errorf("invalid metric name: %w", err)
	}
	u.ID = name
	u.MType = metrics.Type(chi.URLParam(u.Request, "type"))
	value := chi.URLParam(u.Request, "value")

	switch u.MType {
	case metrics.Counter:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return http.StatusBadRequest, fmt.Errorf("value must be an int64: %w", err)
		}
		u.Delta = &v
	case metrics.Gauge:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return http.StatusBadRequest, fmt.Errorf("value must be an float64: %w", err)
		}
		u.Value = &v
	default:
		return http.StatusBadRequest, fmt.Errorf("invalid metric type")
	}

	return http.StatusOK, nil
}
