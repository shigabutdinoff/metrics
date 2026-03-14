package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/metrics"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/update"
	"github.com/shigabutdinoff/metrics/internal/handlers/route/value"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

const (
	DefaultAddress = "localhost:8080"
)

type Server struct {
	Storage storage.Storage
	Address string `env:"ADDRESS"`
	Router  *chi.Mux
}

func New(st storage.Storage) Server {
	r := chi.NewRouter()
	r.Use(middleware.AllowContentType("text/plain"))
	r.Get("/", metrics.Index(st))
	r.Post("/update/{type}/{name}/{value}", update.Store(st))
	r.Get("/value/{type}/{name}", value.Show(st))

	return Server{
		Storage: st,
		Address: DefaultAddress,
		Router:  r,
	}
}

func (s *Server) Run() {
	err := http.ListenAndServe(s.Address, s.Router)
	if err != nil {
		panic(err)
	}
}
