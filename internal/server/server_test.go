package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/shigabutdinoff/metrics/internal/handlers/middleware/compress"
	"github.com/shigabutdinoff/metrics/internal/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	st := storage.NewMemStorage()

	// создаём предустановленный регистратор zap
	logger, err := zap.NewDevelopment()
	if err != nil {
		// вызываем панику, если ошибка
		panic(err)
	}
	defer logger.Sync()

	s := New(st, logger)
	s.setupRoutes()

	if s.Storage != st {
		t.Fatalf("New() несоответствие хранилища")
	}
	if s.Address != DefaultAddress {
		t.Fatalf("New() адрес = %q, ожидается %q", s.Address, DefaultAddress)
	}
	if s.Router == nil {
		t.Fatalf("setupRoutes() роутер равен nil")
	}

	// Smoke-test маршрутов и обработчиков.
	t.Run("GET /", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("GET / статус = %d, ожидается %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("POST /update/gauge", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/update/gauge/temp/12.5", nil)
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("POST /update/gauge статус = %d, ожидается %d", rr.Code, http.StatusOK)
		}

		val := st.GetGauges(context.Background())["temp"]
		if val == nil || *val != 12.5 {
			t.Fatalf("gauge temp не обновлён, получено %v", val)
		}
	})

	t.Run("GET /value/gauge", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/value/gauge/temp", nil)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("GET /value/gauge статус = %d, ожидается %d", rr.Code, http.StatusOK)
		}
		if body := rr.Body.String(); body != "12.5" {
			t.Fatalf("GET /value/gauge тело = %q, ожидается %q", body, "12.5")
		}
	})
}

func TestServer_Run(t *testing.T) {
	t.Run("паникует при ошибке прослушивания", func(t *testing.T) {
		s := &Server{
			Storage: storage.NewMemStorage(),
			Address: "bad",
			Router:  chi.NewRouter(),
		}

		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Run() не вызвал панику при ошибке прослушивания")
			}
		}()

		s.Run()
	})
}

// ...

func TestGzipCompression(t *testing.T) {
	requestBody := `{
        "request": {
            "type": "SimpleUtterance",
            "command": "sudo do something"
        },
        "version": "1.0"
    }`

	// ожидаемое содержимое тела ответа при успешном запросе
	successBody := `{
        "response": {
            "text": "Извините, я пока ничего не умею"
        },
        "version": "1.0"
    }`

	handler := compress.GzipMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.JSONEq(t, requestBody, string(body))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(successBody))
		require.NoError(t, err)
	}))

	srv := httptest.NewServer(handler)
	defer srv.Close()

	t.Run("sends_gzip", func(t *testing.T) {
		buf := bytes.NewBuffer(nil)
		zb := gzip.NewWriter(buf)
		_, err := zb.Write([]byte(requestBody))
		require.NoError(t, err)
		err = zb.Close()
		require.NoError(t, err)

		r := httptest.NewRequest("POST", srv.URL, buf)
		r.RequestURI = ""
		r.Header.Set("Content-Encoding", "gzip")
		r.Header.Set("Accept-Encoding", "")

		resp, err := http.DefaultClient.Do(r)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		defer resp.Body.Close()

		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.JSONEq(t, successBody, string(b))
	})

	t.Run("accepts_gzip", func(t *testing.T) {
		buf := bytes.NewBufferString(requestBody)
		r := httptest.NewRequest("POST", srv.URL, buf)
		r.RequestURI = ""
		r.Header.Set("Accept-Encoding", "gzip")

		resp, err := http.DefaultClient.Do(r)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

		defer resp.Body.Close()

		zr, err := gzip.NewReader(resp.Body)
		require.NoError(t, err)

		b, err := io.ReadAll(zr)
		require.NoError(t, err)

		require.JSONEq(t, successBody, string(b))
	})
}
