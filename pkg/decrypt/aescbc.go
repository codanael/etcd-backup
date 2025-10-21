package decrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"strings"
)

// AESCBCDecryptor handles decryption of AES-CBC encrypted data from etcd
type AESCBCDecryptor struct {
	block   cipher.Block
	keyName string
}

// NewAESCBCDecryptor creates a new AES-CBC decryptor with the given key
func NewAESCBCDecryptor(key []byte, keyName string) (*AESCBCDecryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-CBC requires a 32-byte key, got %d bytes", len(key))
	}

	// Create AES block cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	return &AESCBCDecryptor{
		block:   block,
		keyName: keyName,
	}, nil
}

// Decrypt decrypts data that was encrypted by Kubernetes API server
// Expected format: k8s:enc:aescbc:v1:<keyName>:<encrypted-data>
func (d *AESCBCDecryptor) Decrypt(data []byte) ([]byte, error) {
	// Check if data is encrypted
	prefix := "k8s:enc:aescbc:v1:"
	if !bytes.HasPrefix(data, []byte(prefix)) {
		// Check if it's plaintext (identity provider)
		if bytes.HasPrefix(data, []byte("{")) {
			// Looks like JSON, might be unencrypted
			return data, nil
		}
		return nil, fmt.Errorf("data does not have expected encryption prefix (expected: %s)", prefix)
	}

	// Remove the prefix: k8s:enc:aescbc:v1:
	dataWithoutPrefix := data[len(prefix):]

	// Extract key name and encrypted data
	// Format after prefix: <keyName>:<encrypted-data>
	parts := bytes.SplitN(dataWithoutPrefix, []byte(":"), 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid encrypted data format: expected <keyName>:<encrypted-data>")
	}

	keyName := string(parts[0])
	encryptedPayload := parts[1]

	// Verify key name matches (optional - for informational purposes)
	if d.keyName != "" && keyName != d.keyName {
		return nil, fmt.Errorf("key name mismatch: expected %s, got %s", d.keyName, keyName)
	}

	// Decrypt using AES-CBC
	// First 16 bytes are the IV, rest is ciphertext
	blockSize := d.block.BlockSize()
	if len(encryptedPayload) < blockSize {
		return nil, fmt.Errorf("ciphertext too short (must be at least %d bytes)", blockSize)
	}
	if len(encryptedPayload)%blockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of block size (%d)", blockSize)
	}

	iv := encryptedPayload[:blockSize]
	ciphertext := encryptedPayload[blockSize:]

	// Decrypt
	mode := cipher.NewCBCDecrypter(d.block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS#7 padding
	decrypted, err := removePKCS7Padding(plaintext, blockSize)
	if err != nil {
		return nil, fmt.Errorf("failed to remove padding: %w", err)
	}

	return decrypted, nil
}

// removePKCS7Padding removes PKCS#7 padding from the decrypted data
func removePKCS7Padding(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	paddingLen := int(data[len(data)-1])
	if paddingLen == 0 || paddingLen > blockSize {
		return nil, fmt.Errorf("invalid padding length: %d", paddingLen)
	}

	if paddingLen > len(data) {
		return nil, fmt.Errorf("padding length (%d) exceeds data length (%d)", paddingLen, len(data))
	}

	// Verify all padding bytes are correct
	for i := len(data) - paddingLen; i < len(data); i++ {
		if data[i] != byte(paddingLen) {
			return nil, fmt.Errorf("invalid padding at position %d", i)
		}
	}

	return data[:len(data)-paddingLen], nil
}

// IsEncrypted checks if data appears to be encrypted with aescbc
func IsEncrypted(data []byte) bool {
	return bytes.HasPrefix(data, []byte("k8s:enc:aescbc:v1:"))
}

// ParseEncryptionPrefix parses the encryption prefix and returns provider and key name
func ParseEncryptionPrefix(data []byte) (provider, keyName string, err error) {
	// Expected format: k8s:enc:<provider>:v1:<keyName>:<data>
	str := string(data)
	if !strings.HasPrefix(str, "k8s:enc:") {
		return "", "", fmt.Errorf("not encrypted with k8s encryption")
	}

	parts := strings.Split(str, ":")
	if len(parts) < 5 {
		return "", "", fmt.Errorf("invalid encryption prefix format")
	}

	provider = parts[2] // aescbc, aesgcm, secretbox, kms, etc.
	keyName = parts[4]

	return provider, keyName, nil
}
