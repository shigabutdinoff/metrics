package decrypt

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shigabutdinoff/metrics/pkg/rsacrypt"
)

func TestMiddleware(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`[{"id":"cpu","type":"gauge","value":1.5}]`)
	encrypted, err := rsacrypt.Encrypt(&key.PublicKey, plain)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		key        *rsa.PrivateKey
		body       []byte
		wantStatus int
		wantBody   []byte
		wantCalled bool
	}{
		{name: "nil ключ - тело не трогается", key: nil, body: encrypted, wantStatus: http.StatusOK, wantBody: encrypted, wantCalled: true},
		{name: "шифртекст - хендлер получает открытый текст", key: key, body: encrypted, wantStatus: http.StatusOK, wantBody: plain, wantCalled: true},
		{name: "пустое тело - проходит", key: key, body: []byte{}, wantStatus: http.StatusOK, wantBody: []byte{}, wantCalled: true},
		{name: "не шифртекст - 400", key: key, body: []byte("открытый текст"), wantStatus: http.StatusBadRequest, wantCalled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []byte
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				got, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
			})

			h := Middleware(tt.key)(next)

			req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(tt.body))
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("статус = %d, ожидается %d", rr.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Fatalf("хендлер вызван = %v, ожидается %v", called, tt.wantCalled)
			}
			if called && !bytes.Equal(got, tt.wantBody) {
				t.Fatalf("хендлер получил тело %q, ожидается %q", got, tt.wantBody)
			}
		})
	}
}
