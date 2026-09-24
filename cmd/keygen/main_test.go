package main

import (
	"bytes"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/shigabutdinoff/metrics/pkg/rsacrypt"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()

	if err := run(dir, 2048); err != nil {
		t.Fatalf("run() ошибка = %v", err)
	}

	pub, err := rsacrypt.LoadPublicKey(filepath.Join(dir, "cert.pem"))
	if err != nil {
		t.Fatalf("LoadPublicKey() ошибка = %v", err)
	}
	priv, err := rsacrypt.LoadPrivateKey(filepath.Join(dir, "private.pem"))
	if err != nil {
		t.Fatalf("LoadPrivateKey() ошибка = %v", err)
	}

	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "private.pem"))
	if err != nil {
		t.Fatalf("LoadX509KeyPair() ошибка = %v", err)
	}
	if err = pair.Leaf.VerifyHostname("localhost"); err != nil {
		t.Fatalf("VerifyHostname(localhost) ошибка = %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "private.pem"))
	if err != nil {
		t.Fatalf("Stat() ошибка = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("права private.pem = %o, ожидается 600", perm)
	}

	message := []byte("secret message")
	encrypted, err := rsacrypt.Encrypt(pub, message)
	if err != nil {
		t.Fatalf("Encrypt() ошибка = %v", err)
	}
	decrypted, err := rsacrypt.Decrypt(priv, encrypted)
	if err != nil {
		t.Fatalf("Decrypt() ошибка = %v", err)
	}
	if !bytes.Equal(message, decrypted) {
		t.Fatalf("Decrypt() = %q, ожидается %q", decrypted, message)
	}
}

func TestRun_BadDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	if err := run(missing, 2048); err == nil {
		t.Fatal("ожидалась ошибка записи сертификата")
	}
}
