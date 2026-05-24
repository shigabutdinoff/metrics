package hash

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
)

const Header = "HashSHA256"

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

func (rw *responseWriter) flush() {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	data := rw.buf.Bytes()
	mac := hmac.New(sha256.New, rw.key)
	mac.Write(data)
	rw.ResponseWriter.Header().Set(Header, hex.EncodeToString(mac.Sum(nil)))
	rw.ResponseWriter.WriteHeader(rw.statusCode)
	_, _ = rw.ResponseWriter.Write(data)
}

// Middleware проверяет HashSHA256 входящих запросов и подписывает исходящие ответы.
func Middleware(key *string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k := *key
			if k == "" {
				next.ServeHTTP(w, r)
				return
			}
			keyBytes := []byte(k)

			if r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))

				gotHash := r.Header.Get(Header)
				if gotHash != "" {
					mac := hmac.New(sha256.New, keyBytes)
					mac.Write(body)
					wantHash := hex.EncodeToString(mac.Sum(nil))
					if !hmac.Equal([]byte(gotHash), []byte(wantHash)) {
						http.Error(w, "hash mismatch", http.StatusBadRequest)
						return
					}
				}
			}

			rw := &responseWriter{ResponseWriter: w, key: keyBytes}
			next.ServeHTTP(rw, r)
			rw.flush()
		})
	}
}
