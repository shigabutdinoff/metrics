package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestWithLogging(t *testing.T) {
	const target = "/update/gauge/alloc/1"

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int64
		wantSize   int64
	}{
		{
			name: "пишет статус и размер ответа",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte("создано"))
			},
			wantStatus: http.StatusCreated,
			wantSize:   int64(len("создано")),
		},
		{
			name: "суммирует размер нескольких записей",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ab"))
				_, _ = w.Write([]byte("cde"))
			},
			wantStatus: http.StatusOK,
			wantSize:   5,
		},
		{
			name: "без WriteHeader статус в журнале нулевой",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("ok"))
			},
			wantStatus: 0,
			wantSize:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)

			req := httptest.NewRequest(http.MethodPost, target, nil)
			rr := httptest.NewRecorder()

			WithLogging(zap.New(core))(tt.handler).ServeHTTP(rr, req)

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("записей в журнале = %d, ожидается 1", len(entries))
			}

			fields := entries[0].ContextMap()
			if got := fields["uri"]; got != target {
				t.Fatalf("uri = %v, ожидается %q", got, target)
			}
			if got := fields["method"]; got != http.MethodPost {
				t.Fatalf("method = %v, ожидается %q", got, http.MethodPost)
			}
			if got := fields["status"]; got != tt.wantStatus {
				t.Fatalf("status = %v, ожидается %d", got, tt.wantStatus)
			}
			if got := fields["size"]; got != tt.wantSize {
				t.Fatalf("size = %v, ожидается %d", got, tt.wantSize)
			}
			if _, ok := fields["duration"]; !ok {
				t.Fatal("в журнале нет поля duration")
			}
		})
	}
}
