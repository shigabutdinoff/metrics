package persistent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	m "github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

// Service сохраняет метрики хранилища в файл и читает их обратно.
type Service struct {
	st   storage.Storage
	path string
	log  *zap.Logger
}

// New создаёт сервис, работающий с файлом по пути path.
func New(st storage.Storage, path string, log *zap.Logger) *Service {
	return &Service{st: st, path: path, log: log}
}

// Save пишет метрики во временный файл и переименовывает его.
func (s *Service) Save() error {
	ctx := context.Background()
	gauges := s.st.GetGauges(ctx)
	counters := s.st.GetCounters(ctx)
	out := make([]m.Metrics, 0, len(gauges)+len(counters))
	for name, v := range gauges {
		out = append(out, m.Metrics{ID: name, MType: m.Gauge, Value: v})
	}
	for name, d := range counters {
		out = append(out, m.Metrics{ID: name, MType: m.Counter, Delta: d})
	}

	if dir := filepath.Dir(s.path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, s.path)
}

// Load читает метрики из файла, отсутствие файла не ошибка.
func (s *Service) Load() error {
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	var list []m.Metrics
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, it := range list {
		switch it.MType {
		case m.Gauge:
			s.st.SetGauge(context.Background(), it.ID, it.Value)
		case m.Counter:
			s.st.AddCounter(context.Background(), it.ID, it.Delta)
		}
	}
	return nil
}
