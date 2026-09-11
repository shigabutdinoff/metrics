package reqbody

import (
	"bytes"
	"errors"
	"io"
	"net/http"
)

// MaxBodySize предел размера тела запроса в байтах.
const MaxBodySize = 10 << 20

type body struct {
	bytes.Reader
	data []byte
}

func (*body) Close() error { return nil }

// Read читает тело с лимитом 10 МБ и возвращает его же в r.Body.
func Read(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if b, ok := r.Body.(*body); ok {
		b.Reset(b.data)
		return b.data, true
	}
	if r.Body == nil || r.Body == http.NoBody {
		return nil, true
	}

	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodySize))
	if err != nil {
		respond(w, err, "не удалось прочитать тело запроса")
		return nil, false
	}
	b := &body{data: data}
	b.Reset(data)
	r.Body = b

	return data, true
}

// Error отвечает на ошибку разбора тела: 413 при превышении лимита.
func Error(w http.ResponseWriter, err error) {
	respond(w, err, err.Error())
}

func respond(w http.ResponseWriter, err error, msg string) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		http.Error(w, "тело запроса слишком велико", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, msg, http.StatusBadRequest)
}
