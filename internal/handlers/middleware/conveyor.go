package middleware

import "net/http"

// Middleware обёртка над HTTP-обработчиком.
type Middleware func(http.Handler) http.Handler

// Conveyor навешивает middlewares на h: первый выполняется последним.
func Conveyor(h http.Handler, middlewares ...Middleware) http.Handler {
	for _, middleware := range middlewares {
		h = middleware(h)
	}
	return h
}
