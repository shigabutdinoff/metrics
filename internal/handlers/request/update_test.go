package request

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/shigabutdinoff/metrics/internal/model/metrics"
)

func TestUpdate_Validate(t *testing.T) {
	tests := []struct {
		name      string
		typeValue string
		nameValue string
		rawValue  string
		wantCode  int
		wantErr   bool
		wantDelta *int64
		wantGauge *float64
	}{
		{
			name:      "valid counter",
			typeValue: string(metrics.Counter),
			nameValue: "requests",
			rawValue:  "42",
			wantCode:  http.StatusOK,
			wantErr:   false,
			wantDelta: int64Ptr(42),
		},
		{
			name:      "valid gauge",
			typeValue: string(metrics.Gauge),
			nameValue: "alloc",
			rawValue:  "3.14",
			wantCode:  http.StatusOK,
			wantErr:   false,
			wantGauge: float64Ptr(3.14),
		},
		{
			name:      "missing name",
			typeValue: string(metrics.Gauge),
			nameValue: "   ",
			rawValue:  "1",
			wantCode:  http.StatusNotFound,
			wantErr:   true,
		},
		{
			name:      "invalid type",
			typeValue: "histogram",
			nameValue: "alloc",
			rawValue:  "1",
			wantCode:  http.StatusBadRequest,
			wantErr:   true,
		},
		{
			name:      "invalid counter value",
			typeValue: string(metrics.Counter),
			nameValue: "requests",
			rawValue:  "abc",
			wantCode:  http.StatusBadRequest,
			wantErr:   true,
		},
		{
			name:      "invalid gauge value",
			typeValue: string(metrics.Gauge),
			nameValue: "alloc",
			rawValue:  "abc",
			wantCode:  http.StatusBadRequest,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/update", nil)
			routeCtx := chi.NewRouteContext()
			routeCtx.URLParams.Add("type", tt.typeValue)
			routeCtx.URLParams.Add("name", tt.nameValue)
			routeCtx.URLParams.Add("value", tt.rawValue)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

			u := &Update{Request: req}
			got, err := u.Validate()

			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantCode {
				t.Fatalf("Validate() code = %d, want %d", got, tt.wantCode)
			}

			if err == nil {
				if u.ID != tt.nameValue {
					t.Fatalf("ID = %q, want %q", u.ID, tt.nameValue)
				}
				if string(u.MType) != tt.typeValue {
					t.Fatalf("MType = %q, want %q", u.MType, tt.typeValue)
				}
				if tt.wantDelta != nil {
					if u.Delta == nil || *u.Delta != *tt.wantDelta {
						t.Fatalf("Delta = %v, want %d", u.Delta, *tt.wantDelta)
					}
				}
				if tt.wantGauge != nil {
					if u.Value == nil || *u.Value != *tt.wantGauge {
						t.Fatalf("Value = %v, want %f", u.Value, *tt.wantGauge)
					}
				}
			}
		})
	}
}

func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }
