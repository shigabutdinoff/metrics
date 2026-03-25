package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/compress"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/logging"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/metrics"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/update"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/value"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"go.uber.org/zap"
)

const (
	DefaultAddress = "localhost:8080"
)

type Server struct {
	Storage storage.Storage
	Address string `env:"ADDRESS"`
	Router  *chi.Mux
	Logger  *zap.Logger
}

func New(st storage.Storage, logger *zap.Logger) Server {
	r := chi.NewRouter()
	r.Use(logging.WithLogging(logger))
	r.Group(func(r chi.Router) {
		r.Use(middleware.AllowContentType("text/plain"))
		r.Use(compress.GzipMiddleware())
		r.Get("/", metrics.Index(st))
		r.Post("/update/{type}/{name}/{value}", update.StoreTextPlain(st))
		r.Get("/value/{type}/{name}", value.ShowTextPlain(st))
	})
	r.Group(func(r chi.Router) {
		r.Use(middleware.AllowContentType("application/json"))
		r.Use(compress.GzipMiddleware())
		r.Post("/update/", update.StoreApplicationJSON(st))
		r.Post("/value/", value.ShowApplicationJSON(st))
	})

	return Server{
		Storage: st,
		Address: DefaultAddress,
		Router:  r,
		Logger:  logger,
	}
}

func (s *Server) Run() {
	err := http.ListenAndServe(s.Address, s.Router)
	if err != nil {
		s.Logger.Fatal("Failed to start server", zap.Error(err))
		panic(err)
	}
}
