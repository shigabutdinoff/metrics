package request

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	models "github.com/Shigabutdinoff/metrics/internal/model"
)

type Update struct {
	*http.Request
	models.Metrics
}

func (u *Update) Validate() (int, error) {
	u.ID = u.PathValue("name")
	u.MType = u.PathValue("type")
	value := u.PathValue("value")

	if strings.TrimSpace(u.ID) == "" {
		return http.StatusNotFound, fmt.Errorf("name is required")
	}

	switch u.MType {
	case models.Counter:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return http.StatusBadRequest, fmt.Errorf("value must be an int64: %w", err)
		}
		u.Delta = &v
	case models.Gauge:
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
