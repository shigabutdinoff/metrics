package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/compress"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/hash"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/logging"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/healthcheck"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/metrics"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/update"
	updatesRoute "github.com/shigabutdinoff/metrics/internal/handlers/route/updates"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/value"
	"github.com/shigabutdinoff/metrics/internal/repository/database"
	"github.com/shigabutdinoff/metrics/internal/service/mservice"
	"github.com/shigabutdinoff/metrics/internal/service/persistent"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

const (
	DefaultAddress         = "localhost:8080"
	DefaultStoreInterval   = 300
	DefaultFileStoragePath = ""
	DefaultRestore         = true
	DefaultDatabaseDSN     = ""
	DefaultKey             = ""
)

type Server struct {
	Storage         storage.Storage
	Address         string `env:"ADDRESS"`
	Router          *chi.Mux
	Logger          *zap.Logger
	StoreInterval   int    `env:"STORE_INTERVAL"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
	Restore         bool   `env:"RESTORE"`
	DatabaseDSN     string `env:"DATABASE_DSN"`
	Key             string `env:"KEY"`
	onChange        func()
	Database        *sql.DB
}

func New(st storage.Storage, logger *zap.Logger) *Server {
	s := &Server{
		Storage:         st,
		Address:         DefaultAddress,
		Logger:          logger,
		StoreInterval:   DefaultStoreInterval,
		FileStoragePath: DefaultFileStoragePath,
		Restore:         DefaultRestore,
		DatabaseDSN:     DefaultDatabaseDSN,
		Key:             DefaultKey,
	}

	r := chi.NewRouter()
	r.Use(logging.WithLogging(logger))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
			if req.Method == http.MethodPost && strings.HasPrefix(req.URL.Path, "/update") {
				if s.onChange != nil {
					s.onChange()
				}
			}
		})
	})
	r.Group(func(r chi.Router) {
		r.Use(middleware.AllowContentType("text/plain"))
		r.Use(compress.GzipMiddleware())
		r.Use(hash.Middleware(&s.Key))
		r.Get("/", metrics.Index(s.Storage))
		r.Post("/update/{type}/{name}/{value}", update.StoreTextPlain(s.Storage))
		r.Get("/value/{type}/{name}", value.ShowTextPlain(s.Storage))
	})
	r.Group(func(r chi.Router) {
		r.Use(middleware.AllowContentType("application/json"))
		r.Use(compress.GzipMiddleware())
		r.Use(hash.Middleware(&s.Key))
		r.Post("/update/", update.StoreApplicationJSON(s.Storage))
		r.Post("/updates/", updatesRoute.StoreApplicationJSONBatch(s.Storage, s.Logger))
		r.Post("/value/", value.ShowApplicationJSON(s.Storage))
	})
	r.Get("/ping", healthcheck.Ping(func() *sql.DB {
		return s.Database
	}))
	s.Router = r
	return s
}

func (s *Server) Run() {
	ps := persistent.New(s.Storage, s.FileStoragePath, s.Logger)
	if err := s.initDatabaseOrRestore(ps); err != nil {
		s.Logger.Warn("Инициализация хранилищ завершилась с ошибкой", zap.Error(err))
	}

	if s.Database != nil {
		defer func(db *sql.DB) {
			if err := db.Close(); err != nil {
				panic(err)
			}
		}(s.Database)
	}

	s.configurePersistence(ps)

	if err := http.ListenAndServe(s.Address, s.Router); err != nil {
		s.Logger.Fatal("Failed to start server", zap.Error(err))
		panic(err)
	}
}

func (s *Server) initDatabaseOrRestore(ps *persistent.Service) error {
	db, err := database.Connection(s.DatabaseDSN)
	if err != nil {
		s.Logger.Warn("Не удалось открыть соединение с БД", zap.Error(err))
		if s.Restore && s.FileStoragePath != "" {
			if loadErr := ps.Load(); loadErr != nil {
				s.Logger.Warn("Не удалось восстановить метрики", zap.Error(loadErr))
				return errors.Join(err, loadErr)
			}
		}
		return err
	}

	s.Database = db

	m, mErr := migrate.New("file://migrations", s.DatabaseDSN)
	if mErr != nil {
		s.Logger.Warn("Не удалось инициализировать миграции", zap.Error(mErr))
		return mErr
	}

	if upErr := m.Up(); upErr != nil {
		if !errors.Is(upErr, migrate.ErrNoChange) {
			s.Logger.Warn("Ошибка применения миграций", zap.Error(upErr))
			return upErr
		}
		s.Logger.Info("Миграции: нет изменений")
		return nil
	}

	s.Logger.Info("Миграции успешно применены")
	return nil
}

func (s *Server) configurePersistence(ps *persistent.Service) {
	if s.StoreInterval <= 0 {
		s.onChange = func() {
			if s.Database != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := s.saveToDB(ctx); err != nil {
					s.Logger.Warn("Не удалось сохранить метрики в БД (sync)", zap.Error(err))
				}
			} else if s.FileStoragePath != "" {
				if err := s.saveToFile(ps); err != nil {
					s.Logger.Warn("Не удалось сохранить метрики в файл (sync)", zap.Error(err))
				}
			}
		}
		return
	}

	ticker := time.NewTicker(time.Duration(s.StoreInterval) * time.Second)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			if s.Database != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				if err := s.saveToDB(ctx); err != nil {
					s.Logger.Warn("Не удалось сохранить метрики в БД (tick)", zap.Error(err))
				}
				cancel()
			} else if s.FileStoragePath != "" {
				if err := s.saveToFile(ps); err != nil {
					s.Logger.Warn("Не удалось сохранить метрики в файл (tick)", zap.Error(err))
				}
			}
		}
	}()
}

func (s *Server) saveToDB(ctx context.Context) error {
	repo := database.NewRepository(s.Database)
	op := func() error { return mservice.Upsert(ctx, repo, s.Storage) }
	return withRetry(op, isRetriablePGError, s.Logger)
}

func (s *Server) saveToFile(ps *persistent.Service) error {
	return ps.Save()
}

func isRetriablePGError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	msg := strings.ToUpper(err.Error())
	if strings.Contains(msg, "SQLSTATE 08") {
		return true
	}

	if strings.Contains(msg, "SQLSTATE 40001") ||
		strings.Contains(msg, "SQLSTATE 40P01") ||
		strings.Contains(msg, "SQLSTATE 57P01") ||
		strings.Contains(msg, "SQLSTATE 53300") {
		return true
	}

	return false
}

func withRetry(fn func() error, shouldRetry func(error) bool, logger *zap.Logger) error {
	delays := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}
	var err error
	if err = fn(); err == nil {
		return nil
	}
	for i, d := range delays {
		if !shouldRetry(err) {
			return err
		}
		if logger != nil {
			logger.Info("Повтор операции после ошибки", zap.Error(err), zap.Int("attempt", i+1), zap.Duration("sleep", d))
		}
		time.Sleep(d)
		if err = fn(); err == nil {
			return nil
		}
	}
	return err
}
