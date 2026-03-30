package persist

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	m "github.com/shigabutdinoff/metrics/internal/model/metrics"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

type Service struct {
	st   storage.Storage
	path string
	log  *zap.Logger
}

func New(st storage.Storage, path string, log *zap.Logger) *Service {
	return &Service{st: st, path: path, log: log}
}

func (s *Service) Save() error {
	var out []m.Metrics
	ctx := context.Background()
	for name, v := range s.st.GetGauges(ctx) {
		out = append(out, m.Metrics{ID: name, MType: m.Gauge, Value: v})
	}
	for name, d := range s.st.GetCounters(ctx) {
		out = append(out, m.Metrics{ID: name, MType: m.Counter, Delta: d})
	}

	if dir := filepath.Dir(s.path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	tmp := s.path + ".json"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

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
