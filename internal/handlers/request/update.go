package request

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
)

type Update struct {
	*http.Request
	metrics.Metrics
}

func (u *Update) Validate() (int, error) {
	rawName := chi.URLParam(u.Request, "name")
	name, err := url.PathUnescape(rawName)
	if err != nil || strings.TrimSpace(name) == "" {
		return http.StatusNotFound, fmt.Errorf("неверное название метрики: %w", err)
	}
	u.ID = name
	u.MType = metrics.Type(chi.URLParam(u.Request, "type"))
	value := chi.URLParam(u.Request, "value")

	switch u.MType {
	case metrics.Counter:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return http.StatusBadRequest, fmt.Errorf("значение должно быть int64: %w", err)
		}
		u.Delta = &v
	case metrics.Gauge:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return http.StatusBadRequest, fmt.Errorf("значение должно быть float64: %w", err)
		}
		u.Value = &v
	default:
		return http.StatusBadRequest, fmt.Errorf("неверный тип метрики")
	}

	return http.StatusOK, nil
}
