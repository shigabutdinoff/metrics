package middleware

import "net/http"

// EnsureMethodIsPost принимает параметром Handler и возвращает тоже Handler.
func EnsureMethodIsPost(next http.Handler) http.Handler {
	// получаем Handler приведением типа http.HandlerFunc
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// этот обработчик принимает только запросы, отправленные методом POST
		if r.Method != http.MethodPost {
			http.Error(w, "Допустимы только POST-запросы!", http.StatusMethodNotAllowed)
			return
		}
		// замыкание: используем ServeHTTP следующего хендлера
		next.ServeHTTP(w, r)
	})
}
