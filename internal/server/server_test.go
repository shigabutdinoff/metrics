package server

import (
	"net"
	"testing"
	"time"

	"github.com/Shigabutdinoff/metrics/internal/storage"
)

func TestNew(t *testing.T) {
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		t.Skipf("cannot reserve :8080 for panic-path test: %v", err)
	}
	defer ln.Close()

	panicCh := make(chan any, 1)
	go func() {
		defer func() { panicCh <- recover() }()
		New(storage.NewMemStorage(), ":8080")
	}()

	select {
	case p := <-panicCh:
		if p == nil {
			t.Fatal("New() did not panic on ListenAndServe error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for New() to return panic")
	}
}
