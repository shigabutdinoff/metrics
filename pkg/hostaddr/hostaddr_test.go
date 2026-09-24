package hostaddr

import "testing"

func TestHost(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{name: "IPv4 с портом", addr: "10.0.0.5:34567", want: "10.0.0.5"},
		{name: "IPv6 с портом", addr: "[::1]:8080", want: "::1"},
		{name: "без порта", addr: "10.0.0.5", want: "10.0.0.5"},
		{name: "пустой", addr: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Host(tt.addr); got != tt.want {
				t.Fatalf("Host(%q) = %q, ожидается %q", tt.addr, got, tt.want)
			}
		})
	}
}
