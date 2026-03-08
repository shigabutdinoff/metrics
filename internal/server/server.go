package server

import (
	"net/http"

	"github.com/Shigabutdinoff/metrics/internal/handlers/route/metrics"
	"github.com/Shigabutdinoff/metrics/internal/handlers/route/update"
	"github.com/Shigabutdinoff/metrics/internal/handlers/route/value"
	"github.com/Shigabutdinoff/metrics/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func New(st storage.Storage, addr string) {
	r := chi.NewRouter()
	r.Use(middleware.AllowContentType("text/plain"))
	r.Get("/", metrics.Index(st))
	r.Post("/update/{type}/{name}/{value}", update.Store(st))
	r.Get("/value/{type}/{name}", value.Show(st))

	err := http.ListenAndServe(addr, r)
	if err != nil {
		panic(err)
	}
}
