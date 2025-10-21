package decrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestNewAESCBCDecryptor(t *testing.T) {
	tests := []struct {
		name      string
		key       []byte
		keyName   string
		wantError bool
	}{
		{
			name:      "Valid 32-byte key",
			key:       make([]byte, 32),
			keyName:   "key1",
			wantError: false,
		},
		{
			name:      "Invalid key length - too short",
			key:       make([]byte, 16),
			keyName:   "key1",
			wantError: true,
		},
		{
			name:      "Invalid key length - too long",
			key:       make([]byte, 64),
			keyName:   "key1",
			wantError: true,
		},
		{
			name:      "Empty key",
			key:       []byte{},
			keyName:   "key1",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decryptor, err := NewAESCBCDecryptor(tt.key, tt.keyName)
			if tt.wantError {
				if err == nil {
					t.Errorf("NewAESCBCDecryptor() expected error, got nil")
				}
				if decryptor != nil {
					t.Errorf("NewAESCBCDecryptor() expected nil decryptor on error, got %v", decryptor)
				}
			} else {
				if err != nil {
					t.Errorf("NewAESCBCDecryptor() unexpected error: %v", err)
				}
				if decryptor == nil {
					t.Errorf("NewAESCBCDecryptor() expected decryptor, got nil")
				}
			}
		})
	}
}

func TestRemovePKCS7Padding(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		blockSize int
		want      []byte
		wantError bool
	}{
		{
			name:      "Valid padding - 1 byte",
			data:      []byte("hello world\x01"),
			blockSize: 16,
			want:      []byte("hello world"),
			wantError: false,
		},
		{
			name:      "Valid padding - 5 bytes",
			data:      []byte("hello world\x05\x05\x05\x05\x05"),
			blockSize: 16,
			want:      []byte("hello world"),
			wantError: false,
		},
		{
			name:      "Valid padding - full block",
			data:      []byte("hello world12345\x10\x10\x10\x10\x10\x10\x10\x10\x10\x10\x10\x10\x10\x10\x10\x10"),
			blockSize: 16,
			want:      []byte("hello world12345"),
			wantError: false,
		},
		{
			name:      "Invalid padding - wrong value",
			data:      []byte("hello world\x05\x05\x05\x05\x06"),
			blockSize: 16,
			want:      nil,
			wantError: true,
		},
		{
			name:      "Invalid padding - zero",
			data:      []byte("hello world\x00"),
			blockSize: 16,
			want:      nil,
			wantError: true,
		},
		{
			name:      "Invalid padding - exceeds block size",
			data:      []byte("hello world\x20"),
			blockSize: 16,
			want:      nil,
			wantError: true,
		},
		{
			name:      "Empty data",
			data:      []byte{},
			blockSize: 16,
			want:      nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := removePKCS7Padding(tt.data, tt.blockSize)
			if tt.wantError {
				if err == nil {
					t.Errorf("removePKCS7Padding() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("removePKCS7Padding() unexpected error: %v", err)
				}
				if !bytes.Equal(got, tt.want) {
					t.Errorf("removePKCS7Padding() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "Valid encrypted prefix",
			data: []byte("k8s:enc:aescbc:v1:key1:encrypted-data"),
			want: true,
		},
		{
			name: "Plaintext JSON",
			data: []byte(`{"kind":"Secret"}`),
			want: false,
		},
		{
			name: "Different encryption provider",
			data: []byte("k8s:enc:aesgcm:v1:key1:data"),
			want: false,
		},
		{
			name: "Empty data",
			data: []byte{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEncrypted(tt.data); got != tt.want {
				t.Errorf("IsEncrypted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseEncryptionPrefix(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		wantProvider string
		wantKeyName  string
		wantError    bool
	}{
		{
			name:         "Valid aescbc prefix",
			data:         []byte("k8s:enc:aescbc:v1:key1:data"),
			wantProvider: "aescbc",
			wantKeyName:  "key1",
			wantError:    false,
		},
		{
			name:         "Valid aesgcm prefix",
			data:         []byte("k8s:enc:aesgcm:v1:mykey:data"),
			wantProvider: "aesgcm",
			wantKeyName:  "mykey",
			wantError:    false,
		},
		{
			name:         "Valid secretbox prefix",
			data:         []byte("k8s:enc:secretbox:v1:key2:data"),
			wantProvider: "secretbox",
			wantKeyName:  "key2",
			wantError:    false,
		},
		{
			name:         "No encryption prefix",
			data:         []byte("plain text data"),
			wantProvider: "",
			wantKeyName:  "",
			wantError:    true,
		},
		{
			name:         "Invalid format",
			data:         []byte("k8s:enc:aescbc"),
			wantProvider: "",
			wantKeyName:  "",
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, keyName, err := ParseEncryptionPrefix(tt.data)
			if tt.wantError {
				if err == nil {
					t.Errorf("ParseEncryptionPrefix() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ParseEncryptionPrefix() unexpected error: %v", err)
				}
				if provider != tt.wantProvider {
					t.Errorf("ParseEncryptionPrefix() provider = %v, want %v", provider, tt.wantProvider)
				}
				if keyName != tt.wantKeyName {
					t.Errorf("ParseEncryptionPrefix() keyName = %v, want %v", keyName, tt.wantKeyName)
				}
			}
		})
	}
}

// Helper function to encrypt data using AES-CBC with PKCS#7 padding (like Kubernetes does)
func encryptTestData(key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Add PKCS#7 padding
	blockSize := block.BlockSize()
	paddingLen := blockSize - (len(plaintext) % blockSize)
	padding := bytes.Repeat([]byte{byte(paddingLen)}, paddingLen)
	paddedPlaintext := append(plaintext, padding...)

	// Generate random IV
	iv := make([]byte, blockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	// Encrypt
	mode := cipher.NewCBCEncrypter(block, iv)
	ciphertext := make([]byte, len(paddedPlaintext))
	mode.CryptBlocks(ciphertext, paddedPlaintext)

	// Return IV + ciphertext
	return append(iv, ciphertext...), nil
}

func TestDecrypt(t *testing.T) {
	// Generate a test key
	testKey := make([]byte, 32)
	_, err := rand.Read(testKey)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	testKeyName := "testkey1"
	plaintext := []byte(`{"kind":"Secret","apiVersion":"v1","metadata":{"name":"test"}}`)

	// Encrypt the plaintext
	encryptedData, err := encryptTestData(testKey, plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt test data: %v", err)
	}

	// Create the full encrypted format: k8s:enc:aescbc:v1:keyName:encryptedData
	fullEncrypted := append([]byte("k8s:enc:aescbc:v1:"+testKeyName+":"), encryptedData...)

	tests := []struct {
		name      string
		key       []byte
		keyName   string
		data      []byte
		want      []byte
		wantError bool
	}{
		{
			name:      "Successful decryption",
			key:       testKey,
			keyName:   testKeyName,
			data:      fullEncrypted,
			want:      plaintext,
			wantError: false,
		},
		{
			name:      "Wrong key",
			key:       make([]byte, 32), // Different key
			keyName:   testKeyName,
			data:      fullEncrypted,
			want:      nil,
			wantError: true,
		},
		{
			name:      "Wrong key name",
			key:       testKey,
			keyName:   "wrongkey",
			data:      fullEncrypted,
			want:      nil,
			wantError: true,
		},
		{
			name:      "Plaintext JSON (identity provider)",
			key:       testKey,
			keyName:   testKeyName,
			data:      []byte(`{"kind":"Secret"}`),
			want:      []byte(`{"kind":"Secret"}`),
			wantError: false,
		},
		{
			name:      "Invalid prefix",
			key:       testKey,
			keyName:   testKeyName,
			data:      []byte("invalid:prefix:data"),
			want:      nil,
			wantError: true,
		},
		{
			name:      "Truncated encrypted data",
			key:       testKey,
			keyName:   testKeyName,
			data:      []byte("k8s:enc:aescbc:v1:testkey1:short"),
			want:      nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decryptor, err := NewAESCBCDecryptor(tt.key, tt.keyName)
			if err != nil {
				t.Fatalf("NewAESCBCDecryptor() unexpected error: %v", err)
			}

			got, err := decryptor.Decrypt(tt.data)
			if tt.wantError {
				if err == nil {
					t.Errorf("Decrypt() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Decrypt() unexpected error: %v", err)
				}
				if !bytes.Equal(got, tt.want) {
					t.Errorf("Decrypt() = %q, want %q", got, tt.want)
				}
			}
		})
	}
}

// Benchmark decryption performance
func BenchmarkDecrypt(b *testing.B) {
	testKey := make([]byte, 32)
	rand.Read(testKey)

	plaintext := []byte(`{"kind":"Secret","apiVersion":"v1","metadata":{"name":"test","namespace":"default"},"data":{"password":"c2VjcmV0"}}`)
	encryptedData, _ := encryptTestData(testKey, plaintext)
	fullEncrypted := append([]byte("k8s:enc:aescbc:v1:key1:"), encryptedData...)

	decryptor, _ := NewAESCBCDecryptor(testKey, "key1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decryptor.Decrypt(fullEncrypted)
		if err != nil {
			b.Fatalf("Decrypt failed: %v", err)
		}
	}
}

// Test round-trip encryption/decryption
func TestRoundTrip(t *testing.T) {
	testKey := make([]byte, 32)
	_, err := rand.Read(testKey)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	testCases := [][]byte{
		[]byte(""),                    // Empty
		[]byte("a"),                   // Single character
		[]byte("Hello, World!"),       // Simple text
		bytes.Repeat([]byte("x"), 16), // Exactly one block
		bytes.Repeat([]byte("y"), 32), // Exactly two blocks
		bytes.Repeat([]byte("z"), 17), // More than one block, not aligned
		[]byte(`{"kind":"Secret","apiVersion":"v1","metadata":{"name":"mysecret","namespace":"default"},"type":"Opaque","data":{"username":"YWRtaW4=","password":"MWYyZDFlMmU2N2Rm"}}`),
	}

	decryptor, err := NewAESCBCDecryptor(testKey, "key1")
	if err != nil {
		t.Fatalf("NewAESCBCDecryptor() error: %v", err)
	}

	for i, plaintext := range testCases {
		t.Run(string(rune(i)), func(t *testing.T) {
			// Encrypt
			encrypted, err := encryptTestData(testKey, plaintext)
			if err != nil {
				t.Fatalf("encryptTestData() error: %v", err)
			}

			// Add Kubernetes prefix
			fullEncrypted := append([]byte("k8s:enc:aescbc:v1:key1:"), encrypted...)

			// Decrypt
			decrypted, err := decryptor.Decrypt(fullEncrypted)
			if err != nil {
				t.Fatalf("Decrypt() error: %v", err)
			}

			// Verify
			if !bytes.Equal(decrypted, plaintext) {
				t.Errorf("Round-trip failed: got %q, want %q", decrypted, plaintext)
			}
		})
	}
}

// Test base64 key handling
func TestBase64KeyHandling(t *testing.T) {
	// Generate a key and encode it
	rawKey := make([]byte, 32)
	rand.Read(rawKey)
	base64Key := base64.StdEncoding.EncodeToString(rawKey)

	// Decode the key
	decodedKey, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		t.Fatalf("Failed to decode base64 key: %v", err)
	}

	// Create decryptor with decoded key
	decryptor, err := NewAESCBCDecryptor(decodedKey, "key1")
	if err != nil {
		t.Fatalf("NewAESCBCDecryptor() error: %v", err)
	}

	if decryptor == nil {
		t.Errorf("Expected decryptor, got nil")
	}

	// Verify key length
	if len(decodedKey) != 32 {
		t.Errorf("Decoded key length = %d, want 32", len(decodedKey))
	}
}
