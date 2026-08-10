package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

type AESGCMProtector struct {
	aead cipher.AEAD
}

func NewAESGCMProtector(key []byte) (*AESGCMProtector, error) {
	if len(key) == 0 {
		return nil, errors.New("token encryption key is required")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create token cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create token protector: %w", err)
	}
	return &AESGCMProtector{aead: aead}, nil
}

func (p *AESGCMProtector) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate token nonce: %w", err)
	}
	return p.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func (p *AESGCMProtector) Decrypt(ciphertext []byte) (string, error) {
	if len(ciphertext) < p.aead.NonceSize() {
		return "", errors.New("invalid encrypted token")
	}
	nonce := ciphertext[:p.aead.NonceSize()]
	plaintext, err := p.aead.Open(nil, nonce, ciphertext[p.aead.NonceSize():], nil)
	if err != nil {
		return "", errors.New("decrypt token: authentication failed")
	}
	return string(plaintext), nil
}
