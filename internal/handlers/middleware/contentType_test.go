package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnsureContentTypeIsTextPlain(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantStatus  int
		wantCalled  bool
	}{
		{
			name:        "accepts exact text/plain",
			contentType: "text/plain",
			wantStatus:  http.StatusOK,
			wantCalled:  true,
		},
		{
			name:        "accepts text/plain with charset",
			contentType: "text/plain; charset=utf-8",
			wantStatus:  http.StatusOK,
			wantCalled:  true,
		},
		{
			name:        "rejects non text/plain",
			contentType: "application/json",
			wantStatus:  http.StatusUnsupportedMediaType,
			wantCalled:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			h := EnsureContentTypeIsTextPlain(next)
			req := httptest.NewRequest(http.MethodPost, "/update/gauge/name/1", nil)
			req.Header.Set("Content-Type", tt.contentType)
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
