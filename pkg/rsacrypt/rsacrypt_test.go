package rsacrypt

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var testKey = mustGenerateKey()

func mustGenerateKey() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
}

func TestEncryptDecrypt(t *testing.T) {
	size := testKey.Size()
	chunk := size - overhead

	tests := []struct {
		name       string
		n          int
		wantBlocks int
	}{
		{name: "пусто", n: 0, wantBlocks: 0},
		{name: "один байт", n: 1, wantBlocks: 1},
		{name: "ровно блок", n: chunk, wantBlocks: 1},
		{name: "блок и байт", n: chunk + 1, wantBlocks: 2},
		{name: "три блока", n: 3 * chunk, wantBlocks: 3},
		{name: "три блока с хвостом", n: 3*chunk + 5, wantBlocks: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.n)
			if _, err := rand.Read(data); err != nil {
				t.Fatal(err)
			}

			encrypted, err := Encrypt(&testKey.PublicKey, data)
			if err != nil {
				t.Fatalf("Encrypt() ошибка = %v", err)
			}
			if len(encrypted) != tt.wantBlocks*size {
				t.Fatalf("len(encrypted) = %d, ожидается %d блоков по %d байт", len(encrypted), tt.wantBlocks, size)
			}

			decrypted, err := Decrypt(testKey, encrypted)
			if err != nil {
				t.Fatalf("Decrypt() ошибка = %v", err)
			}
			if !bytes.Equal(decrypted, data) {
				t.Fatalf("Decrypt() вернул %d байт, не совпадающих с исходными %d", len(decrypted), len(data))
			}
		})
	}
}

func TestDecrypt_Errors(t *testing.T) {
	size := testKey.Size()

	tests := []struct {
		name string
		data []byte
	}{
		{name: "длина не кратна размеру ключа", data: make([]byte, size+1)},
		{name: "блок без паддинга OAEP", data: make([]byte, size)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decrypt(testKey, tt.data); err == nil {
				t.Fatal("ожидалась ошибка расшифровки")
			}
		})
	}
}

func TestEncrypt_SmallKey(t *testing.T) {
	// синтетический ключ на 512 бит, GenerateKey такие уже не выдаёт
	pub := &rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), 511), E: 65537}

	if _, err := Encrypt(pub, []byte("данные")); err == nil {
		t.Fatal("ожидалась ошибка для слишком малого ключа")
	}
}

func writePEM(t *testing.T, typ string, der []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func certDER(t *testing.T, pub, priv any) []byte {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestLoadPublicKey(t *testing.T) {
	pub := &testKey.PublicKey

	pkix, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		typ     string
		der     []byte
		wantErr bool
	}{
		{name: "сертификат", typ: "CERTIFICATE", der: certDER(t, pub, testKey)},
		{name: "PKIX вместо сертификата", typ: "PUBLIC KEY", der: pkix, wantErr: true},
		{name: "сертификат с ключом не RSA", typ: "CERTIFICATE", der: certDER(t, &ecdsaKey.PublicKey, ecdsaKey), wantErr: true},
		{name: "битый DER", typ: "CERTIFICATE", der: []byte("junk"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadPublicKey(writePEM(t, tt.typ, tt.der))
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadPublicKey() ошибка = %v, ожидается ошибка %v", err, tt.wantErr)
			}
			if !tt.wantErr && !got.Equal(pub) {
				t.Fatal("LoadPublicKey() вернул другой ключ")
			}
		})
	}
}

func TestLoadPrivateKey(t *testing.T) {
	pkcs8, err := x509.MarshalPKCS8PrivateKey(testKey)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		typ     string
		der     []byte
		wantErr bool
	}{
		{name: "PKCS1", typ: "RSA PRIVATE KEY", der: x509.MarshalPKCS1PrivateKey(testKey)},
		{name: "PKCS8 вместо PKCS1", typ: "PRIVATE KEY", der: pkcs8, wantErr: true},
		{name: "битый DER", typ: "RSA PRIVATE KEY", der: []byte("junk"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadPrivateKey(writePEM(t, tt.typ, tt.der))
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadPrivateKey() ошибка = %v, ожидается ошибка %v", err, tt.wantErr)
			}
			if !tt.wantErr && !got.Equal(testKey) {
				t.Fatal("LoadPrivateKey() вернул другой ключ")
			}
		})
	}
}

func TestLoad_BadFiles(t *testing.T) {
	notPEM := filepath.Join(t.TempDir(), "not.pem")
	if err := os.WriteFile(notPEM, []byte("не PEM"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing.pem")

	for _, path := range []string{notPEM, missing} {
		if _, err := LoadPublicKey(path); err == nil {
			t.Errorf("LoadPublicKey(%q): ожидалась ошибка", path)
		}
		if _, err := LoadPrivateKey(path); err == nil {
			t.Errorf("LoadPrivateKey(%q): ожидалась ошибка", path)
		}
	}
}
