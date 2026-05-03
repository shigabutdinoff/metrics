package healthcheck

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

func Ping(getDB func() *sql.DB) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		db := getDB()
		if db == nil {
			res.WriteHeader(http.StatusInternalServerError)
			_, _ = res.Write([]byte("Не удалось открыть соединение с БД"))
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), 1*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
			_, _ = res.Write([]byte("Не удалось проверить соединение с БД"))
			return
		}

		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte("Соединение с БД установлено"))
	}
}
