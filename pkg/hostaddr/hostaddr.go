package hostaddr

import "net"

// Host возвращает хост из addr вида host:port.
// Адрес без порта возвращается как есть.
func Host(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
