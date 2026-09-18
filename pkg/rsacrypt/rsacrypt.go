package rsacrypt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// Служебные байты паддинга OAEP с SHA-256 в каждом блоке.
const overhead = 2*sha256.Size + 2

// LoadPublicKey читает публичный ключ RSA из PEM-файла с сертификатом.
func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	certificateBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	certificatePemBlock, _ := pem.Decode(certificateBytes)
	if certificatePemBlock == nil {
		return nil, errors.New("сертификат не найден")
	}
	certificate, err := x509.ParseCertificate(certificatePemBlock.Bytes)
	if err != nil {
		return nil, err
	}
	publicKey, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("ключ %T не RSA", certificate.PublicKey)
	}
	return publicKey, nil
}

// LoadPrivateKey читает приватный ключ RSA из PEM-файла в формате PKCS1.
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	privateKeyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	privateKeyPemBlock, _ := pem.Decode(privateKeyBytes)
	if privateKeyPemBlock == nil {
		return nil, errors.New("приватный ключ не найден")
	}
	return x509.ParsePKCS1PrivateKey(privateKeyPemBlock.Bytes)
}

// Encrypt шифрует data публичным ключом RSA блоками.
func Encrypt(pub *rsa.PublicKey, data []byte) ([]byte, error) {
	h := sha256.New()
	chunk := pub.Size() - overhead
	if chunk <= 0 {
		return nil, errors.New("ключ слишком мал для OAEP с SHA-256")
	}
	out := make([]byte, 0, (len(data)+chunk-1)/chunk*pub.Size())
	for len(data) > 0 {
		n := min(chunk, len(data))
		block, err := rsa.EncryptOAEP(h, rand.Reader, pub, data[:n], nil)
		if err != nil {
			return nil, err
		}
		out = append(out, block...)
		data = data[n:]
	}
	return out, nil
}

// Decrypt расшифровывает data приватным ключом блоками длиной priv.Size().
func Decrypt(priv *rsa.PrivateKey, data []byte) ([]byte, error) {
	h := sha256.New()
	size := priv.Size()
	if len(data)%size != 0 {
		return nil, errors.New("длина данных не кратна размеру ключа")
	}
	out := make([]byte, 0, len(data)/size*(size-overhead))
	for len(data) > 0 {
		block, err := rsa.DecryptOAEP(h, nil, priv, data[:size], nil)
		if err != nil {
			return nil, err
		}
		out = append(out, block...)
		data = data[size:]
	}
	return out, nil
}
