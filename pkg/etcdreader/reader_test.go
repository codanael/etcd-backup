package etcdreader

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/server/v3/mvcc/buckets"
)

// createTestSnapshot creates a test etcd snapshot database with MVCC encoding
func createTestSnapshot(t *testing.T, data map[string][]byte) string {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-snapshot.db")

	// Create a new bolt database
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Create the "key" bucket and populate it with MVCC-encoded data
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(buckets.Key.Name())
		if err != nil {
			return err
		}

		rev := int64(1)
		for key, value := range data {
			// Create MVCC revision key (8 bytes main + 1 byte separator + 8 bytes sub)
			revBytes := make([]byte, 17)
			binary.BigEndian.PutUint64(revBytes[0:8], uint64(rev))
			revBytes[8] = '_'
			binary.BigEndian.PutUint64(revBytes[9:17], 0) // sub revision = 0

			// Create MVCC KeyValue protobuf
			kv := &mvccpb.KeyValue{
				Key:            []byte(key),
				Value:          value,
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
		t.Fatalf("Failed to populate test database: %v", err)
	}

	return dbPath
}

func TestNewReader(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() string
		wantError bool
	}{
		{
			name: "Valid snapshot file",
			setup: func() string {
				return createTestSnapshot(t, map[string][]byte{
					"/registry/secrets/default/test": []byte("data"),
				})
			},
			wantError: false,
		},
		{
			name: "Non-existent file",
			setup: func() string {
				return "/nonexistent/path/to/snapshot.db"
			},
			wantError: true,
		},
		{
			name: "Invalid database file",
			setup: func() string {
				tmpDir := t.TempDir()
				invalidFile := filepath.Join(tmpDir, "invalid.db")
				os.WriteFile(invalidFile, []byte("not a valid bolt db"), 0600)
				return invalidFile
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()
			reader, err := NewReader(path)

			if tt.wantError {
				if err == nil {
					t.Errorf("NewReader() expected error, got nil")
				}
				if reader != nil {
					reader.Close()
					t.Errorf("NewReader() expected nil reader on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewReader() unexpected error: %v", err)
				}
				if reader == nil {
					t.Errorf("NewReader() expected reader, got nil")
				} else {
					defer reader.Close()
				}
			}
		})
	}
}

func TestReaderClose(t *testing.T) {
	dbPath := createTestSnapshot(t, map[string][]byte{
		"/registry/secrets/default/test": []byte("data"),
	})

	reader, err := NewReader(dbPath)
	if err != nil {
		t.Fatalf("NewReader() error: %v", err)
	}

	// Close should succeed
	err = reader.Close()
	if err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}

	// Second close should also succeed (idempotent)
	err = reader.Close()
	if err != nil {
		t.Errorf("Second Close() unexpected error: %v", err)
	}
}

func TestReaderGet(t *testing.T) {
	testData := map[string][]byte{
		"/registry/secrets/default/secret1":     []byte("secret1-data"),
		"/registry/secrets/kube-system/secret2": []byte("secret2-data"),
		"/registry/configmaps/default/config1":  []byte("config1-data"),
	}

	dbPath := createTestSnapshot(t, testData)
	reader, err := NewReader(dbPath)
	if err != nil {
		t.Fatalf("NewReader() error: %v", err)
	}
	defer reader.Close()

	tests := []struct {
		name      string
		key       string
		want      []byte
		wantError bool
	}{
		{
			name:      "Existing secret in default namespace",
			key:       "/registry/secrets/default/secret1",
			want:      []byte("secret1-data"),
			wantError: false,
		},
		{
			name:      "Existing secret in kube-system namespace",
			key:       "/registry/secrets/kube-system/secret2",
			want:      []byte("secret2-data"),
			wantError: false,
		},
		{
			name:      "Existing configmap",
			key:       "/registry/configmaps/default/config1",
			want:      []byte("config1-data"),
			wantError: false,
		},
		{
			name:      "Non-existent key",
			key:       "/registry/secrets/default/nonexistent",
			want:      nil,
			wantError: true,
		},
		{
			name:      "Empty key",
			key:       "",
			want:      nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reader.Get(tt.key)
			if tt.wantError {
				if err == nil {
					t.Errorf("Get() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Get() unexpected error: %v", err)
				}
				if string(got) != string(tt.want) {
					t.Errorf("Get() = %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestReaderListSecrets(t *testing.T) {
	testData := map[string][]byte{
		"/registry/secrets/default/secret1":       []byte("data1"),
		"/registry/secrets/default/secret2":       []byte("data2"),
		"/registry/secrets/kube-system/secret3":   []byte("data3"),
		"/registry/configmaps/default/config1":    []byte("config-data"),
		"/registry/pods/default/pod1":             []byte("pod-data"),
		"/registry/secrets/production/db-secret":  []byte("db-data"),
		"/registry/secrets/production/api-secret": []byte("api-data"),
	}

	dbPath := createTestSnapshot(t, testData)
	reader, err := NewReader(dbPath)
	if err != nil {
		t.Fatalf("NewReader() error: %v", err)
	}
	defer reader.Close()

	secrets, err := reader.ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets() error: %v", err)
	}

	// Expected secrets (should only list items under /registry/secrets/)
	expectedSecrets := []string{
		"/registry/secrets/default/secret1",
		"/registry/secrets/default/secret2",
		"/registry/secrets/kube-system/secret3",
		"/registry/secrets/production/api-secret",
		"/registry/secrets/production/db-secret",
	}

	if len(secrets) != len(expectedSecrets) {
		t.Errorf("ListSecrets() returned %d secrets, want %d", len(secrets), len(expectedSecrets))
	}

	// Convert to map for easy lookup
	secretMap := make(map[string]bool)
	for _, s := range secrets {
		secretMap[s] = true
	}

	for _, expected := range expectedSecrets {
		if !secretMap[expected] {
			t.Errorf("ListSecrets() missing expected secret: %s", expected)
		}
	}

	// Ensure no non-secret items are included
	for _, secret := range secrets {
		if secret == "/registry/configmaps/default/config1" ||
			secret == "/registry/pods/default/pod1" {
			t.Errorf("ListSecrets() included non-secret item: %s", secret)
		}
	}
}

func TestReaderListSecretsEmpty(t *testing.T) {
	// Create a snapshot with no secrets
	testData := map[string][]byte{
		"/registry/configmaps/default/config1": []byte("config-data"),
		"/registry/pods/default/pod1":          []byte("pod-data"),
	}

	dbPath := createTestSnapshot(t, testData)
	reader, err := NewReader(dbPath)
	if err != nil {
		t.Fatalf("NewReader() error: %v", err)
	}
	defer reader.Close()

	secrets, err := reader.ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets() error: %v", err)
	}

	if len(secrets) != 0 {
		t.Errorf("ListSecrets() on empty secrets = %d items, want 0", len(secrets))
	}
}

func TestReaderListAll(t *testing.T) {
	testData := map[string][]byte{
		"/registry/secrets/default/secret1":    []byte("data1"),
		"/registry/configmaps/default/config1": []byte("config-data"),
		"/registry/pods/default/pod1":          []byte("pod-data"),
	}

	dbPath := createTestSnapshot(t, testData)
	reader, err := NewReader(dbPath)
	if err != nil {
		t.Fatalf("NewReader() error: %v", err)
	}
	defer reader.Close()

	all, err := reader.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}

	if len(all) != len(testData) {
		t.Errorf("ListAll() returned %d keys, want %d", len(all), len(testData))
	}

	// Verify all keys are present
	keyMap := make(map[string]bool)
	for _, key := range all {
		keyMap[key] = true
	}

	for expectedKey := range testData {
		if !keyMap[expectedKey] {
			t.Errorf("ListAll() missing expected key: %s", expectedKey)
		}
	}
}

func TestReaderWithRealSecretFormat(t *testing.T) {
	// Simulate real Kubernetes secret data (encrypted)
	encryptedSecret := []byte("k8s:enc:aescbc:v1:key1:\x00\x01\x02\x03\x04encrypted-data-here")
	plaintextSecret := []byte(`{"kind":"Secret","apiVersion":"v1","metadata":{"name":"test"}}`)

	testData := map[string][]byte{
		"/registry/secrets/default/encrypted-secret": encryptedSecret,
		"/registry/secrets/default/plaintext-secret": plaintextSecret,
	}

	dbPath := createTestSnapshot(t, testData)
	reader, err := NewReader(dbPath)
	if err != nil {
		t.Fatalf("NewReader() error: %v", err)
	}
	defer reader.Close()

	// Test encrypted secret
	encrypted, err := reader.Get("/registry/secrets/default/encrypted-secret")
	if err != nil {
		t.Errorf("Get() encrypted secret error: %v", err)
	}
	expectedPrefix := "k8s:enc:aescbc:v1:key1:"
	if len(encrypted) < len(expectedPrefix) {
		t.Errorf("Encrypted secret too short: got %d bytes", len(encrypted))
	} else if string(encrypted[:len(expectedPrefix)]) != expectedPrefix {
		t.Errorf("Encrypted secret prefix = %q, want %q", string(encrypted[:len(expectedPrefix)]), expectedPrefix)
	}

	// Test plaintext secret
	plaintext, err := reader.Get("/registry/secrets/default/plaintext-secret")
	if err != nil {
		t.Errorf("Get() plaintext secret error: %v", err)
	}
	if string(plaintext) != string(plaintextSecret) {
		t.Errorf("Plaintext secret = %q, want %q", plaintext, plaintextSecret)
	}
}

// Benchmark reader operations
func BenchmarkReaderGet(b *testing.B) {
	testData := map[string][]byte{
		"/registry/secrets/default/secret1": []byte("secret-data"),
	}

	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench-snapshot.db")

	db, _ := bolt.Open(dbPath, 0600, nil)
	db.Update(func(tx *bolt.Tx) error {
		bucket, _ := tx.CreateBucketIfNotExists([]byte("key"))
		for k, v := range testData {
			bucket.Put([]byte(k), v)
		}
		return nil
	})
	db.Close()

	reader, _ := NewReader(dbPath)
	defer reader.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := reader.Get("/registry/secrets/default/secret1")
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

func BenchmarkReaderListSecrets(b *testing.B) {
	testData := make(map[string][]byte)
	for i := 0; i < 100; i++ {
		key := string(rune(i))
		testData["/registry/secrets/default/secret"+key] = []byte("data")
	}

	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench-snapshot.db")

	db, _ := bolt.Open(dbPath, 0600, nil)
	db.Update(func(tx *bolt.Tx) error {
		bucket, _ := tx.CreateBucketIfNotExists([]byte("key"))
		for k, v := range testData {
			bucket.Put([]byte(k), v)
		}
		return nil
	})
	db.Close()

	reader, _ := NewReader(dbPath)
	defer reader.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := reader.ListSecrets()
		if err != nil {
			b.Fatalf("ListSecrets failed: %v", err)
		}
	}
}
