package hash

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"go.uber.org/zap"
)

const header = "HashSHA256"

// maxBodySize 10 МБ
const maxBodySize = 10 << 20

type responseWriter struct {
	http.ResponseWriter
	buf        bytes.Buffer
	key        []byte
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.buf.Write(b)
}

func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func (rw *responseWriter) flush() error {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	data := rw.buf.Bytes()
	mac := hmac.New(sha256.New, rw.key)
	mac.Write(data)
	rw.ResponseWriter.Header().Set(header, hex.EncodeToString(mac.Sum(nil)))
	rw.ResponseWriter.WriteHeader(rw.statusCode)
	_, err := rw.ResponseWriter.Write(data)
	return err
}

// Middleware проверяет HashSHA256 входящих запросов и подписывает исходящие ответы.
func Middleware(key string, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			keyBytes := []byte(key)

			if r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "тело запроса слишком велико", http.StatusRequestEntityTooLarge)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))

				gotHash := r.Header.Get(header)
				if gotHash != "" {
					mac := hmac.New(sha256.New, keyBytes)
					mac.Write(body)
					wantHash := hex.EncodeToString(mac.Sum(nil))
					if !hmac.Equal([]byte(gotHash), []byte(wantHash)) {
						http.Error(w, "несовпадение хэша", http.StatusBadRequest)
						return
					}
				}
			}

			rw := &responseWriter{ResponseWriter: w, key: keyBytes}
			next.ServeHTTP(rw, r)
			if err := rw.flush(); err != nil {
				logger.Error("Ошибка записи тела ответа", zap.Error(err))
			}
		})
	}
}
