package trustedsubnet

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// Header заголовок с IP агента, его выставляет агент и читает Middleware.
const Header = "X-Real-IP"

// Middleware пускает запрос, если IP из X-Real-IP или X-Forwarded-For
// входит в subnet, иначе пишет причину в logger и отвечает 403.
// nil подсеть прозрачна.
func Middleware(subnet *net.IPNet, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if subnet == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, err := resolveIP(r)
			if err != nil {
				logger.Warn("IP агента не определён",
					zap.String("remote_addr", r.RemoteAddr), zap.Error(err))
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			if !subnet.Contains(ip) {
				logger.Warn("IP агента вне доверенной подсети",
					zap.String("remote_addr", r.RemoteAddr), zap.Stringer("ip", ip))
				http.Error(w, "IP агента вне доверенной подсети", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func resolveIP(r *http.Request) (net.IP, error) {
	ipStr := r.Header.Get(Header)
	ip := net.ParseIP(ipStr)
	if ip == nil {
		ips := r.Header.Get("X-Forwarded-For")
		ipStrs := strings.Split(ips, ",")
		ipStr = strings.TrimSpace(ipStrs[0])
		ip = net.ParseIP(ipStr)
	}
	if ip == nil {
		return nil, errors.New("IP агента не определён")
	}
	return ip, nil
}
