package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

var (
	secretKeyOnce sync.Once
	secretKey     []byte
	secretKeyErr  error
)

// getSecretKey resolves and caches the encryption key for the lifetime of the
// process. The key is read from the environment or disk only once, instead of
// on every Encrypt/Decrypt call.
func getSecretKey() ([]byte, error) {
	secretKeyOnce.Do(func() {
		secretKey, secretKeyErr = loadSecretKey()
	})
	return secretKey, secretKeyErr
}

func loadSecretKey() ([]byte, error) {
	key := os.Getenv("SECRET_KEY")
	if key != "" {
		if len(key) < 32 {
			return nil, errors.New("SECRET_KEY environment variable must be at least 32 characters")
		}
		return []byte(key[:32]), nil
	}

	secretFile := ".mimic_secret"
	data, err := os.ReadFile(secretFile)
	if err == nil {
		if len(data) >= 32 {
			return data[:32], nil
		}
	}

	// Generate new key
	newKey := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return nil, err
	}

	b64Key := base64.StdEncoding.EncodeToString(newKey)
	if err := os.WriteFile(secretFile, []byte(b64Key), 0600); err != nil {
		return nil, err
	}

	return []byte(b64Key), nil
}

// ValidateSecretKey forces key validation during application startup instead
// of waiting until the first credential is encrypted or decrypted.
func ValidateSecretKey() error {
	_, err := getSecretKey()
	return err
}

// Encrypt string using AES-GCM
func Encrypt(text string) (string, error) {
	key, err := getSecretKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(text), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt string using AES-GCM
func Decrypt(cryptoText string) (string, error) {
	key, err := getSecretKey()
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// HashPassword using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// CheckPasswordHash compares password and hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
