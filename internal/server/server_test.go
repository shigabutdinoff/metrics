package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

func TestNew(t *testing.T) {
	st := storage.NewMemStorage()
	s := New(st)

	if s.Storage != st {
		t.Fatalf("New() storage mismatch")
	}
	if s.Address != DefaultAddress {
		t.Fatalf("New() address = %q, want %q", s.Address, DefaultAddress)
	}
	if s.Router == nil {
		t.Fatalf("New() router is nil")
	}

	// Smoke-test routes and handlers.
	t.Run("GET /", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("GET / status = %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("POST /update/gauge", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/update/gauge/temp/12.5", nil)
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("POST /update/gauge status = %d, want %d", rr.Code, http.StatusOK)
		}

		val := st.GetGauges()["temp"]
		if val == nil || *val != 12.5 {
			t.Fatalf("gauge temp not updated, got %v", val)
		}
	})

	t.Run("GET /value/gauge", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/value/gauge/temp", nil)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("GET /value/gauge status = %d, want %d", rr.Code, http.StatusOK)
		}
		if body := rr.Body.String(); body != "12.5" {
			t.Fatalf("GET /value/gauge body = %q, want %q", body, "12.5")
		}
	})
}

func TestServer_Run(t *testing.T) {
	t.Run("panics on listen error", func(t *testing.T) {
		s := &Server{
			Storage: storage.NewMemStorage(),
			Address: "bad",
			Router:  chi.NewRouter(),
		}

		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Run() did not panic on listen error")
			}
		}()

		s.Run()
	})
}
