package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	customMiddleware "github.com/shigabutdinoff/metrics/internal/handlers/middleware"
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
	r.Use(middleware.AllowContentType("text/plain"))
	r.Use(customMiddleware.WithLogging(logger))
	r.Get("/", metrics.Index(st))
	r.Post("/update/{type}/{name}/{value}", update.Store(st))
	r.Get("/value/{type}/{name}", value.Show(st))

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
