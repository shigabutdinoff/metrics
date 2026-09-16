package hash

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/reqbody"
)

const header = "HashSHA256"

const maxPooledBuffer = 64 << 10

var buffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func putBuffer(buf *bytes.Buffer) {
	if buf.Cap() <= maxPooledBuffer {
		buf.Reset()
		buffers.Put(buf)
	}
}

type responseWriter struct {
	http.ResponseWriter
	buf        *bytes.Buffer
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

// Middleware проверяет подпись запросов и подписывает ответы.
func Middleware(key string, logger *zap.Logger) func(http.Handler) http.Handler {
	keyBytes := []byte(key)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			body, ok := reqbody.Read(w, r)
			if !ok {
				return
			}
			if gotHash := r.Header.Get(header); body != nil && gotHash != "" {
				mac := hmac.New(sha256.New, keyBytes)
				mac.Write(body)
				wantHash := hex.EncodeToString(mac.Sum(nil))
				if !hmac.Equal([]byte(gotHash), []byte(wantHash)) {
					http.Error(w, "несовпадение хэша", http.StatusBadRequest)
					return
				}
			}

			rw := &responseWriter{ResponseWriter: w, key: keyBytes, buf: buffers.Get().(*bytes.Buffer)}
			defer putBuffer(rw.buf)
			next.ServeHTTP(rw, r)
			if err := rw.flush(); err != nil {
				logger.Error("Ошибка записи тела ответа", zap.Error(err))
			}
		})
	}
}
