package server

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/compress"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/logging"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/healthcheck"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/metrics"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/update"
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
		r.Get("/", metrics.Index(s.Storage))
		r.Post("/update/{type}/{name}/{value}", update.StoreTextPlain(s.Storage))
		r.Get("/value/{type}/{name}", value.ShowTextPlain(s.Storage))
	})
	r.Group(func(r chi.Router) {
		r.Use(middleware.AllowContentType("application/json"))
		r.Use(compress.GzipMiddleware())
		r.Post("/update/", update.StoreApplicationJSON(s.Storage))
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
	db, err := database.Connection(s.DatabaseDSN)

	if err != nil {
		s.Logger.Warn("Не удалось открыть соединение с БД", zap.Error(err))

		if s.Restore && s.FileStoragePath != "" {
			if err := ps.Load(); err != nil {
				s.Logger.Warn("Не удалось восстановить метрики", zap.Error(err))
			}
		}
	} else {
		defer func(db *sql.DB) {
			err := db.Close()
			if err != nil {
				panic(err)
			}
		}(s.Database)

		s.Database = db
	}

	if s.StoreInterval <= 0 {
		s.onChange = func() {
			if s.Database != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				// не забываем освободить ресурс
				defer cancel()
				// обращаемся к БД
				if err := mservice.Upsert(ctx, s.Database, s.Storage, s.Logger); err != nil {
					s.Logger.Warn("Не удалось сохранить метрики в БД (sync)", zap.Error(err))
				}
			} else if s.FileStoragePath != "" {
				if err := ps.Save(); err != nil {
					s.Logger.Warn("Не удалось сохранить метрики в файл (sync)", zap.Error(err))
				}
			}
		}
	} else {
		ticker := time.NewTicker(time.Duration(s.StoreInterval) * time.Second)
		go func() {
			defer ticker.Stop()
			for range ticker.C {
				if s.Database != nil {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					if err := mservice.Upsert(ctx, s.Database, s.Storage, s.Logger); err != nil {
						s.Logger.Warn("Не удалось сохранить метрики в БД (tick)", zap.Error(err))
					}
					cancel()
				} else if s.FileStoragePath != "" {
					if err := ps.Save(); err != nil {
						s.Logger.Warn("Не удалось сохранить метрики в файл (tick)", zap.Error(err))
					}
				}
			}
		}()
	}

	err = http.ListenAndServe(s.Address, s.Router)
	if err != nil {
		s.Logger.Fatal("Failed to start server", zap.Error(err))
		panic(err)
	}
}
