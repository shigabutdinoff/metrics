package main

import (
	"net/http"

	"github.com/Shigabutdinoff/metrics/internal/handlers/middleware"
	"github.com/Shigabutdinoff/metrics/internal/handlers/route"
	"github.com/Shigabutdinoff/metrics/internal/storage"
)

func main() {
	st := storage.NewMemStorage()

	mux := http.NewServeMux()
	mux.Handle("/update/{type}/{name}/{value}", middleware.Conveyor(
		route.Update(st),
		middleware.EnsureContentTypeIsTextPlain,
		middleware.EnsureMethodIsPost,
	))

	err := http.ListenAndServe(`:8080`, mux)
	if err != nil {
		panic(err)
	}
}
