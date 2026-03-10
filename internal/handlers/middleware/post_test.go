package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnsureMethodIsPost(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "allows POST",
			method:     http.MethodPost,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "rejects GET",
			method:     http.MethodGet,
			wantStatus: http.StatusMethodNotAllowed,
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			h := EnsureMethodIsPost(next)
			req := httptest.NewRequest(tt.method, "/update/gauge/name/1", nil)
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Fatalf("next called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}
