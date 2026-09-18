package decrypt

import (
	"bytes"
	"crypto/rsa"
	"io"
	"net/http"

	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/reqbody"
	"github.com/shigabutdinoff/metrics/pkg/rsacrypt"
)

// Middleware расшифровывает тело запроса ключом key, nil ключ прозрачен.
func Middleware(key *rsa.PrivateKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, ok := reqbody.Read(w, r)
			if !ok {
				return
			}
			plain, err := rsacrypt.Decrypt(key, body)
			if err != nil {
				http.Error(w, "не удалось расшифровать тело запроса", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(plain))
			r.ContentLength = int64(len(plain))
			next.ServeHTTP(w, r)
		})
	}
}
