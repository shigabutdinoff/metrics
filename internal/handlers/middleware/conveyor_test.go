package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestConveyor(t *testing.T) {
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("возвращает исходный обработчик при пустом списке middleware", func(t *testing.T) {
		h := Conveyor(base)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("статус = %d, ожидается %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("применяет цепочку middleware", func(t *testing.T) {
		var order []string

		m1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "m1-before")
				next.ServeHTTP(w, r)
				order = append(order, "m1-after")
			})
		}
		m2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "m2-before")
				next.ServeHTTP(w, r)
				order = append(order, "m2-after")
			})
		}
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
			w.WriteHeader(http.StatusNoContent)
		})

		h := Conveyor(handler, m1, m2)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(rr, req)

		wantOrder := []string{"m2-before", "m1-before", "handler", "m1-after", "m2-after"}
		if !reflect.DeepEqual(order, wantOrder) {
			t.Fatalf("порядок = %v, ожидается %v", order, wantOrder)
		}
		if rr.Code != http.StatusNoContent {
			t.Fatalf("статус = %d, ожидается %d", rr.Code, http.StatusNoContent)
		}
	})
}
