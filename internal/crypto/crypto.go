// Package crypto implements optional at-rest encryption for SSTable files
// using AES-GCM with key derivation via hand-composed PBKDF2.
// Every primitive (AES, SHA-256, HMAC) is a composed standard-library
// building block — no custom cipher logic.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
)

// PBKDF2 derives a key from a passphrase using the PBKDF2 algorithm,
// hand-composed from crypto/hmac + crypto/sha256 (stdlib primitives only).
// This replaces golang.org/x/crypto/pbkdf2.
func PBKDF2(password, salt []byte, iterations, keyLen int) []byte {
	numBlocks := (keyLen + sha256.Size - 1) / sha256.Size
	dk := make([]byte, 0, numBlocks*sha256.Size)

	for block := 1; block <= numBlocks; block++ {
		dk = append(dk, pbkdf2Block(password, salt, iterations, block)...)
	}

	return dk[:keyLen]
}

func pbkdf2Block(password, salt []byte, iterations, blockNum int) []byte {
	mac := hmac.New(sha256.New, password)

	saltBlock := make([]byte, len(salt)+4)
	copy(saltBlock, salt)
	saltBlock[len(salt)+0] = byte(blockNum >> 24)
	saltBlock[len(salt)+1] = byte(blockNum >> 16)
	saltBlock[len(salt)+2] = byte(blockNum >> 8)
	saltBlock[len(salt)+3] = byte(blockNum)

	mac.Write(saltBlock)
	u := mac.Sum(nil)

	result := make([]byte, len(u))
	copy(result, u)

	for i := 2; i <= iterations; i++ {
		mac.Reset()
		mac.Write(u)
		u = mac.Sum(nil)

		for j := range result {
			result[j] ^= u[j]
		}
	}

	return result
}

// Encryptor provides AES-GCM encryption/decryption for data at rest.
type Encryptor struct {
	gcm  cipher.AEAD
	salt []byte
}

const (
	SaltSize    = 32
	pbkdf2Iters = 100000
	aesKeySize  = 32 // AES-256
)

// NewEncryptor creates an encryptor from a user passphrase.
func NewEncryptor(passphrase string) (*Encryptor, error) {
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("crypto: generate salt: %w", err)
	}

	return newEncryptorWithSalt(passphrase, salt)
}

// NewEncryptorWithSalt creates an encryptor from a passphrase and existing salt.
func NewEncryptorWithSalt(passphrase string, salt []byte) (*Encryptor, error) {
	return newEncryptorWithSalt(passphrase, salt)
}

func newEncryptorWithSalt(passphrase string, salt []byte) (*Encryptor, error) {
	key := PBKDF2([]byte(passphrase), salt, pbkdf2Iters, aesKeySize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}

	return &Encryptor{gcm: gcm, salt: salt}, nil
}

// Encrypt encrypts plaintext using AES-GCM.
// Returns: salt (32B) || nonce || ciphertext+tag
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	ciphertext := e.gcm.Seal(nil, nonce, plaintext, nil)

	result := make([]byte, len(e.salt)+len(nonce)+len(ciphertext))
	copy(result, e.salt)
	copy(result[len(e.salt):], nonce)
	copy(result[len(e.salt)+len(nonce):], ciphertext)

	return result, nil
}

// Decrypt decrypts data encrypted with Encrypt.
func Decrypt(passphrase string, data []byte) ([]byte, error) {
	if len(data) < SaltSize+12 {
		return nil, errors.New("crypto: data too short")
	}

	salt := data[:SaltSize]
	enc, err := NewEncryptorWithSalt(passphrase, salt)
	if err != nil {
		return nil, err
	}

	nonceSize := enc.gcm.NonceSize()
	if len(data) < SaltSize+nonceSize {
		return nil, errors.New("crypto: data too short for nonce")
	}

	nonce := data[SaltSize : SaltSize+nonceSize]
	ciphertext := data[SaltSize+nonceSize:]

	plaintext, err := enc.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt failed (wrong passphrase?): %w", err)
	}

	return plaintext, nil
}

// Salt returns the salt used by this encryptor.
func (e *Encryptor) Salt() []byte {
	result := make([]byte, len(e.salt))
	copy(result, e.salt)
	return result
}

// EncryptFile encrypts srcPath contents and writes to dstPath.
func EncryptFile(passphrase, srcPath, dstPath string) error {
	enc, err := NewEncryptor(passphrase)
	if err != nil {
		return err
	}

	plaintext, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("crypto: read source: %w", err)
	}

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		return err
	}

	return os.WriteFile(dstPath, ciphertext, 0644)
}

// DecryptFile decrypts srcPath contents and writes to dstPath.
func DecryptFile(passphrase, srcPath, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("crypto: read encrypted file: %w", err)
	}

	plaintext, err := Decrypt(passphrase, data)
	if err != nil {
		return err
	}

	return os.WriteFile(dstPath, plaintext, 0644)
}
