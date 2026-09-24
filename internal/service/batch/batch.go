package batch

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

// Ошибки проверки пачки, которые возвращает Update.
var (
	// ErrEmpty пачка без метрик.
	ErrEmpty = errors.New("отсутствуют метрики")
	// ErrName пустое имя метрики.
	ErrName = errors.New("неверное название метрики")
	// ErrType неизвестный тип метрики.
	ErrType = errors.New("неверный тип метрики")
	// ErrValue нет значения, либо gauge равен NaN или Inf.
	ErrValue = errors.New("неверное значение метрики")
)

// Update проверяет всю пачку и только потом пишет метрики в st по одной.
// При ошибке проверки ничего не пишет. После записи шлёт событие аудита
// с адресом ip в n, если n не nil.
func Update(
	ctx context.Context,
	st storage.Storage,
	n audit.Notifier,
	ip string,
	items []metrics.Metrics,
) error {
	if err := validate(items); err != nil {
		return err
	}

	names := make([]string, 0, len(items))
	for _, it := range items {
		switch it.MType {
		case metrics.Gauge:
			st.SetGauge(ctx, it.ID, it.Value)
		case metrics.Counter:
			st.AddCounter(ctx, it.ID, it.Delta)
		}
		names = append(names, it.ID)
	}

	if n != nil {
		n.Publish(audit.Event{TS: time.Now().Unix(), Metrics: names, IPAddress: ip})
	}
	return nil
}

func validate(items []metrics.Metrics) error {
	if len(items) == 0 {
		return ErrEmpty
	}

	for _, it := range items {
		if strings.TrimSpace(it.ID) == "" {
			return ErrName
		}

		switch it.MType {
		case metrics.Gauge:
			if it.Value == nil || math.IsNaN(*it.Value) || math.IsInf(*it.Value, 0) {
				return ErrValue
			}
		case metrics.Counter:
			if it.Delta == nil {
				return ErrValue
			}
		default:
			return ErrType
		}
	}
	return nil
}
