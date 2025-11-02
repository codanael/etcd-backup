package test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/codanael/etcd-secret-reader/pkg/decrypt"
	"github.com/codanael/etcd-secret-reader/pkg/etcdreader"
	bolt "go.etcd.io/bbolt"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/server/v3/mvcc/buckets"
)

// KubernetesSecret represents a Kubernetes Secret object
type KubernetesSecret struct {
	Kind       string            `json:"kind"`
	APIVersion string            `json:"apiVersion"`
	Metadata   map[string]string `json:"metadata"`
	Type       string            `json:"type"`
	Data       map[string]string `json:"data"`
}

// encryptData encrypts data using AES-CBC with PKCS#7 padding
func encryptData(key []byte, plaintext []byte) ([]byte, error) {
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

// createEncryptedSecret creates a Kubernetes secret and encrypts it
func createEncryptedSecret(t *testing.T, key []byte, keyName, namespace, name string, secretData map[string]string) []byte {
	t.Helper()

	// Create Kubernetes Secret object
	secret := KubernetesSecret{
		Kind:       "Secret",
		APIVersion: "v1",
		Metadata: map[string]string{
			"name":      name,
			"namespace": namespace,
		},
		Type: "Opaque",
		Data: secretData,
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("Failed to marshal secret: %v", err)
	}

	// Encrypt the JSON data
	encrypted, err := encryptData(key, jsonData)
	if err != nil {
		t.Fatalf("Failed to encrypt data: %v", err)
	}

	// Add Kubernetes encryption prefix
	prefix := "k8s:enc:aescbc:v1:" + keyName + ":"
	return append([]byte(prefix), encrypted...)
}

// createTestSnapshotWithSecrets creates a test etcd snapshot with encrypted secrets
func createTestSnapshotWithSecrets(t *testing.T, key []byte, keyName string, secrets map[string]map[string]string) string {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-snapshot.db")

	// Create bolt database
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Populate with encrypted secrets using MVCC encoding
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(buckets.Key.Name())
		if err != nil {
			return err
		}

		rev := int64(1)
		for path, data := range secrets {
			// Parse namespace and name from path
			// Expected format: /registry/secrets/<namespace>/<name>
			parts := []rune(path)
			var namespace, name string
			slashCount := 0
			nsStart, nsEnd, nameStart := -1, -1, -1

			for i, c := range parts {
				if c == '/' {
					slashCount++
					if slashCount == 3 {
						nsStart = i + 1
					} else if slashCount == 4 {
						nsEnd = i
						nameStart = i + 1
					}
				}
			}

			if nsStart > 0 && nsEnd > nsStart {
				namespace = string(parts[nsStart:nsEnd])
			}
			if nameStart > 0 {
				name = string(parts[nameStart:])
			}

			// Create and encrypt secret
			encryptedData := createEncryptedSecret(t, key, keyName, namespace, name, data)

			// Create MVCC revision key
			revBytes := make([]byte, 17)
			binary.BigEndian.PutUint64(revBytes[0:8], uint64(rev))
			revBytes[8] = '_'
			binary.BigEndian.PutUint64(revBytes[9:17], 0)

			// Create MVCC KeyValue protobuf
			kv := &mvccpb.KeyValue{
				Key:            []byte(path),
				Value:          encryptedData,
				CreateRevision: rev,
				ModRevision:    rev,
				Version:        1,
			}

			// Marshal to protobuf
			kvBytes, err := kv.Marshal()
			if err != nil {
				return err
			}

			// Store with MVCC revision as key
			if err := bucket.Put(revBytes, kvBytes); err != nil {
				return err
			}

			rev++
		}

		return nil
	})

	if err != nil {
		t.Fatalf("Failed to populate database: %v", err)
	}

	return dbPath
}

func TestEndToEndEncryptionDecryption(t *testing.T) {
	// Generate encryption key
	encryptionKey := make([]byte, 32)
	_, err := rand.Read(encryptionKey)
	if err != nil {
		t.Fatalf("Failed to generate encryption key: %v", err)
	}

	keyName := "mykey"

	// Define test secrets
	testSecrets := map[string]map[string]string{
		"/registry/secrets/default/db-credentials": {
			"username": base64.StdEncoding.EncodeToString([]byte("admin")),
			"password": base64.StdEncoding.EncodeToString([]byte("secret123")),
		},
		"/registry/secrets/kube-system/api-token": {
			"token": base64.StdEncoding.EncodeToString([]byte("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")),
		},
		"/registry/secrets/production/tls-cert": {
			"tls.crt": base64.StdEncoding.EncodeToString([]byte("-----BEGIN CERTIFICATE-----")),
			"tls.key": base64.StdEncoding.EncodeToString([]byte("-----BEGIN PRIVATE KEY-----")),
		},
	}

	// Create snapshot with encrypted secrets
	snapshotPath := createTestSnapshotWithSecrets(t, encryptionKey, keyName, testSecrets)

	// Open the snapshot
	reader, err := etcdreader.NewReader(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to open snapshot: %v", err)
	}
	defer reader.Close()

	// Create decryptor
	decryptor, err := decrypt.NewAESCBCDecryptor(encryptionKey, keyName)
	if err != nil {
		t.Fatalf("Failed to create decryptor: %v", err)
	}

	// Test: List all secrets
	t.Run("ListSecrets", func(t *testing.T) {
		secrets, err := reader.ListSecrets()
		if err != nil {
			t.Fatalf("ListSecrets() error: %v", err)
		}

		if len(secrets) != len(testSecrets) {
			t.Errorf("ListSecrets() returned %d secrets, want %d", len(secrets), len(testSecrets))
		}

		secretMap := make(map[string]bool)
		for _, s := range secrets {
			secretMap[s] = true
		}

		for expectedPath := range testSecrets {
			if !secretMap[expectedPath] {
				t.Errorf("ListSecrets() missing expected secret: %s", expectedPath)
			}
		}
	})

	// Test: Read and decrypt each secret
	for path, expectedData := range testSecrets {
		t.Run("Decrypt_"+path, func(t *testing.T) {
			// Read encrypted data
			encryptedData, err := reader.Get(path)
			if err != nil {
				t.Fatalf("Get() error: %v", err)
			}

			// Decrypt
			decryptedData, err := decryptor.Decrypt(encryptedData)
			if err != nil {
				t.Fatalf("Decrypt() error: %v", err)
			}

			// Parse secret
			var secret KubernetesSecret
			if err := json.Unmarshal(decryptedData, &secret); err != nil {
				t.Fatalf("Failed to unmarshal secret: %v", err)
			}

			// Verify secret data
			if secret.Kind != "Secret" {
				t.Errorf("Secret kind = %q, want %q", secret.Kind, "Secret")
			}

			if secret.Type != "Opaque" {
				t.Errorf("Secret type = %q, want %q", secret.Type, "Opaque")
			}

			// Verify data fields
			for key, expectedValue := range expectedData {
				if secret.Data[key] != expectedValue {
					t.Errorf("Secret data[%s] = %q, want %q", key, secret.Data[key], expectedValue)
				}
			}
		})
	}
}

func TestDecryptionWithWrongKey(t *testing.T) {
	// Generate two different keys
	correctKey := make([]byte, 32)
	wrongKey := make([]byte, 32)
	rand.Read(correctKey)
	rand.Read(wrongKey)

	keyName := "testkey"

	// Create snapshot with secret encrypted using correct key
	testSecrets := map[string]map[string]string{
		"/registry/secrets/default/test-secret": {
			"data": base64.StdEncoding.EncodeToString([]byte("secret-value")),
		},
	}

	snapshotPath := createTestSnapshotWithSecrets(t, correctKey, keyName, testSecrets)

	// Open snapshot
	reader, err := etcdreader.NewReader(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to open snapshot: %v", err)
	}
	defer reader.Close()

	// Try to decrypt with wrong key
	wrongDecryptor, err := decrypt.NewAESCBCDecryptor(wrongKey, keyName)
	if err != nil {
		t.Fatalf("Failed to create decryptor: %v", err)
	}

	encryptedData, err := reader.Get("/registry/secrets/default/test-secret")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	// This should fail because the key is wrong
	_, err = wrongDecryptor.Decrypt(encryptedData)
	if err == nil {
		t.Errorf("Decrypt() with wrong key should fail, but succeeded")
	}

	// Now try with correct key
	correctDecryptor, err := decrypt.NewAESCBCDecryptor(correctKey, keyName)
	if err != nil {
		t.Fatalf("Failed to create correct decryptor: %v", err)
	}

	decryptedData, err := correctDecryptor.Decrypt(encryptedData)
	if err != nil {
		t.Errorf("Decrypt() with correct key failed: %v", err)
	}

	var secret KubernetesSecret
	if err := json.Unmarshal(decryptedData, &secret); err != nil {
		t.Errorf("Failed to unmarshal decrypted secret: %v", err)
	}
}

func TestPlaintextSecretHandling(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-snapshot.db")

	// Create database with plaintext (unencrypted) secret
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	plaintextSecret := KubernetesSecret{
		Kind:       "Secret",
		APIVersion: "v1",
		Metadata: map[string]string{
			"name":      "plaintext-secret",
			"namespace": "default",
		},
		Type: "Opaque",
		Data: map[string]string{
			"key": base64.StdEncoding.EncodeToString([]byte("value")),
		},
	}

	jsonData, _ := json.Marshal(plaintextSecret)

	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(buckets.Key.Name())
		if err != nil {
			return err
		}

		// Create MVCC revision key
		revBytes := make([]byte, 17)
		binary.BigEndian.PutUint64(revBytes[0:8], 1)
		revBytes[8] = '_'
		binary.BigEndian.PutUint64(revBytes[9:17], 0)

		// Create MVCC KeyValue protobuf
		kv := &mvccpb.KeyValue{
			Key:            []byte("/registry/secrets/default/plaintext-secret"),
			Value:          jsonData,
			CreateRevision: 1,
			ModRevision:    1,
			Version:        1,
		}

		// Marshal to protobuf
		kvBytes, err := kv.Marshal()
		if err != nil {
			return err
		}

		return bucket.Put(revBytes, kvBytes)
	})
	db.Close()

	if err != nil {
		t.Fatalf("Failed to populate database: %v", err)
	}

	// Open snapshot
	reader, err := etcdreader.NewReader(dbPath)
	if err != nil {
		t.Fatalf("Failed to open snapshot: %v", err)
	}
	defer reader.Close()

	// Create decryptor (with any key, won't be used for plaintext)
	key := make([]byte, 32)
	decryptor, err := decrypt.NewAESCBCDecryptor(key, "key1")
	if err != nil {
		t.Fatalf("Failed to create decryptor: %v", err)
	}

	// Read plaintext secret
	data, err := reader.Get("/registry/secrets/default/plaintext-secret")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	// Decrypt should recognize it's plaintext and return as-is
	decrypted, err := decryptor.Decrypt(data)
	if err != nil {
		t.Fatalf("Decrypt() plaintext error: %v", err)
	}

	// Should be able to unmarshal directly
	var secret KubernetesSecret
	if err := json.Unmarshal(decrypted, &secret); err != nil {
		t.Fatalf("Failed to unmarshal plaintext secret: %v", err)
	}

	if secret.Metadata["name"] != "plaintext-secret" {
		t.Errorf("Secret name = %q, want %q", secret.Metadata["name"], "plaintext-secret")
	}
}

func TestMultipleSecretsInDifferentNamespaces(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	// Create secrets in multiple namespaces
	testSecrets := map[string]map[string]string{
		"/registry/secrets/default/app-config": {
			"config": base64.StdEncoding.EncodeToString([]byte("app configuration")),
		},
		"/registry/secrets/default/app-secret": {
			"secret": base64.StdEncoding.EncodeToString([]byte("app secret")),
		},
		"/registry/secrets/production/db-creds": {
			"username": base64.StdEncoding.EncodeToString([]byte("prod-user")),
			"password": base64.StdEncoding.EncodeToString([]byte("prod-pass")),
		},
		"/registry/secrets/staging/db-creds": {
			"username": base64.StdEncoding.EncodeToString([]byte("staging-user")),
			"password": base64.StdEncoding.EncodeToString([]byte("staging-pass")),
		},
		"/registry/secrets/kube-system/system-secret": {
			"token": base64.StdEncoding.EncodeToString([]byte("system-token")),
		},
	}

	snapshotPath := createTestSnapshotWithSecrets(t, key, "key1", testSecrets)

	reader, err := etcdreader.NewReader(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to open snapshot: %v", err)
	}
	defer reader.Close()

	decryptor, err := decrypt.NewAESCBCDecryptor(key, "key1")
	if err != nil {
		t.Fatalf("Failed to create decryptor: %v", err)
	}

	// List all secrets
	secrets, err := reader.ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets() error: %v", err)
	}

	// Should find all 5 secrets
	if len(secrets) != 5 {
		t.Errorf("ListSecrets() found %d secrets, want 5", len(secrets))
	}

	// Decrypt each and verify
	for _, secretPath := range secrets {
		encryptedData, err := reader.Get(secretPath)
		if err != nil {
			t.Errorf("Get(%s) error: %v", secretPath, err)
			continue
		}

		decryptedData, err := decryptor.Decrypt(encryptedData)
		if err != nil {
			t.Errorf("Decrypt(%s) error: %v", secretPath, err)
			continue
		}

		var secret KubernetesSecret
		if err := json.Unmarshal(decryptedData, &secret); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", secretPath, err)
			continue
		}

		if secret.Kind != "Secret" {
			t.Errorf("Secret %s kind = %q, want Secret", secretPath, secret.Kind)
		}
	}
}

// Benchmark the complete flow
func BenchmarkEndToEndDecryption(b *testing.B) {
	key := make([]byte, 32)
	rand.Read(key)

	testSecrets := map[string]map[string]string{
		"/registry/secrets/default/test-secret": {
			"data": base64.StdEncoding.EncodeToString([]byte("test-data")),
		},
	}

	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench-snapshot.db")

	// Create snapshot
	db, _ := bolt.Open(dbPath, 0600, nil)
	db.Update(func(tx *bolt.Tx) error {
		bucket, _ := tx.CreateBucketIfNotExists([]byte("key"))

		secret := KubernetesSecret{
			Kind:       "Secret",
			APIVersion: "v1",
			Metadata:   map[string]string{"name": "test-secret", "namespace": "default"},
			Type:       "Opaque",
			Data:       testSecrets["/registry/secrets/default/test-secret"],
		}

		jsonData, _ := json.Marshal(secret)

		// Encrypt
		block, _ := aes.NewCipher(key)
		blockSize := block.BlockSize()
		paddingLen := blockSize - (len(jsonData) % blockSize)
		padding := bytes.Repeat([]byte{byte(paddingLen)}, paddingLen)
		paddedPlaintext := append(jsonData, padding...)
		iv := make([]byte, blockSize)
		rand.Read(iv)
		mode := cipher.NewCBCEncrypter(block, iv)
		ciphertext := make([]byte, len(paddedPlaintext))
		mode.CryptBlocks(ciphertext, paddedPlaintext)
		encrypted := append(iv, ciphertext...)
		fullEncrypted := append([]byte("k8s:enc:aescbc:v1:key1:"), encrypted...)

		bucket.Put([]byte("/registry/secrets/default/test-secret"), fullEncrypted)
		return nil
	})
	db.Close()

	reader, _ := etcdreader.NewReader(dbPath)
	defer reader.Close()

	decryptor, _ := decrypt.NewAESCBCDecryptor(key, "key1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encryptedData, _ := reader.Get("/registry/secrets/default/test-secret")
		decryptedData, _ := decryptor.Decrypt(encryptedData)
		var secret KubernetesSecret
		json.Unmarshal(decryptedData, &secret)
	}
}
