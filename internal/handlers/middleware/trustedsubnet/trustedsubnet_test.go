package trustedsubnet

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestMiddleware(t *testing.T) {
	_, subnet, err := net.ParseCIDR("192.168.0.0/24")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		subnet       *net.IPNet
		realIP       string
		forwardedFor string
		wantStatus   int
		wantLog      string
		wantLogIP    string
	}{
		{name: "nil подсеть - без проверки", subnet: nil, realIP: "", wantStatus: http.StatusOK},
		{name: "IP в подсети - проходит", subnet: subnet, realIP: "192.168.0.69", wantStatus: http.StatusOK},
		{name: "IP вне подсети - 403", subnet: subnet, realIP: "10.0.0.1", wantStatus: http.StatusForbidden, wantLog: "IP агента вне доверенной подсети", wantLogIP: "10.0.0.1"},
		{name: "нет заголовка - 403", subnet: subnet, realIP: "", wantStatus: http.StatusForbidden, wantLog: "IP агента не определён"},
		{name: "не IP - 403", subnet: subnet, realIP: "localhost", wantStatus: http.StatusForbidden, wantLog: "IP агента не определён"},
		{name: "IPv6 при IPv4 подсети - 403", subnet: subnet, realIP: "fe80::1", wantStatus: http.StatusForbidden, wantLog: "IP агента вне доверенной подсети", wantLogIP: "fe80::1"},
		{name: "X-Forwarded-For в подсети - проходит", subnet: subnet, forwardedFor: "192.168.0.69, 10.0.0.1", wantStatus: http.StatusOK},
		{name: "X-Forwarded-For с пробелом перед запятой - проходит", subnet: subnet, forwardedFor: "192.168.0.69 , 10.0.0.1", wantStatus: http.StatusOK},
		{name: "X-Forwarded-For вне подсети - 403", subnet: subnet, forwardedFor: "10.0.0.1, 192.168.0.69", wantStatus: http.StatusForbidden, wantLog: "IP агента вне доверенной подсети", wantLogIP: "10.0.0.1"},
		{name: "X-Real-IP главнее X-Forwarded-For - 403", subnet: subnet, realIP: "10.0.0.1", forwardedFor: "192.168.0.69", wantStatus: http.StatusForbidden, wantLog: "IP агента вне доверенной подсети", wantLogIP: "10.0.0.1"},
		{name: "X-Real-IP не IP, X-Forwarded-For в подсети - проходит", subnet: subnet, realIP: "localhost", forwardedFor: "192.168.0.69", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			core, logs := observer.New(zap.WarnLevel)
			h := Middleware(tt.subnet, zap.New(core))(next)

			req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
			if tt.realIP != "" {
				req.Header.Set(Header, tt.realIP)
			}
			if tt.forwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.forwardedFor)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("статус = %d, ожидается %d", rr.Code, tt.wantStatus)
			}
			if want := tt.wantStatus == http.StatusOK; called != want {
				t.Fatalf("хендлер вызван = %v, ожидается %v", called, want)
			}
			if tt.wantLog == "" {
				if logs.Len() != 0 {
					t.Fatalf("лишние записи в логе: %v", logs.All())
				}
				return
			}
			if body := strings.TrimSpace(rr.Body.String()); body != tt.wantLog {
				t.Fatalf("тело ответа = %q, ожидается %q", body, tt.wantLog)
			}
			entries := logs.FilterMessage(tt.wantLog).All()
			if len(entries) != 1 {
				t.Fatalf("в логе нет %q: %v", tt.wantLog, logs.All())
			}
			if addr := entries[0].ContextMap()["remote_addr"]; addr != req.RemoteAddr {
				t.Fatalf("remote_addr в логе = %v, ожидается %s", addr, req.RemoteAddr)
			}
			if tt.wantLogIP != "" {
				if ip := entries[0].ContextMap()["ip"]; ip != tt.wantLogIP {
					t.Fatalf("ip в логе = %v, ожидается %s", ip, tt.wantLogIP)
				}
			}
		})
	}
}
