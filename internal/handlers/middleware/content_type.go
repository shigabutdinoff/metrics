package middleware

import (
	"net/http"
	"strings"
)

// EnsureContentTypeIsTextPlain принимает параметром Handler и возвращает тоже Handler.
func EnsureContentTypeIsTextPlain(next http.Handler) http.Handler {
	// получаем Handler приведением типа http.HandlerFunc
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// этот обработчик принимает только запросы, отправленные с заголовком Content-Type: text/plain
		contentType := r.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/plain") {
			http.Error(w, "Only text/plain Content-Type are allowed!", http.StatusUnsupportedMediaType)
			return
		}
		// замыкание: используем ServeHTTP следующего хендлера
		next.ServeHTTP(w, r)
	})
}
