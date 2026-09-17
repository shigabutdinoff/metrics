package rsacrypt_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"github.com/shigabutdinoff/metrics/pkg/rsacrypt"
)

// Сообщение длиннее одного блока RSA шифруется и расшифровывается целиком.
func ExampleEncrypt() {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}

	message := bytes.Repeat([]byte("secret message "), 40)
	encryptedMessage, err := rsacrypt.Encrypt(&privateKey.PublicKey, message)
	if err != nil {
		panic(err)
	}

	decryptedMessage, err := rsacrypt.Decrypt(privateKey, encryptedMessage)
	if err != nil {
		panic(err)
	}

	fmt.Println(bytes.Equal(message, decryptedMessage))
	// Output:
	// true
}
