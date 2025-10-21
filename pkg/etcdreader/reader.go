package etcdreader

import (
	"fmt"
	"strings"

	bolt "go.etcd.io/bbolt"
)

// Reader provides access to etcd snapshot data
type Reader struct {
	db *bolt.DB
}

// NewReader opens an etcd snapshot file for reading
func NewReader(snapshotPath string) (*Reader, error) {
	// Open the bbolt database in read-only mode
	db, err := bolt.Open(snapshotPath, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to open snapshot: %w", err)
	}

	return &Reader{db: db}, nil
}

// Close closes the snapshot file
func (r *Reader) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// Get retrieves a value from etcd by key
func (r *Reader) Get(key string) ([]byte, error) {
	var data []byte

	err := r.db.View(func(tx *bolt.Tx) error {
		// etcd stores data in the "key" bucket
		bucket := tx.Bucket([]byte("key"))
		if bucket == nil {
			return fmt.Errorf("key bucket not found in snapshot")
		}

		// Get the value
		val := bucket.Get([]byte(key))
		if val == nil {
			return fmt.Errorf("key not found: %s", key)
		}

		// Copy the data since it's only valid during the transaction
		data = make([]byte, len(val))
		copy(data, val)

		return nil
	})

	return data, err
}

// ListSecrets lists all secrets in the snapshot
func (r *Reader) ListSecrets() ([]string, error) {
	var secrets []string

	err := r.db.View(func(tx *bolt.Tx) error {
		// etcd stores data in the "key" bucket
		bucket := tx.Bucket([]byte("key"))
		if bucket == nil {
			return fmt.Errorf("key bucket not found in snapshot")
		}

		// Iterate through keys
		c := bucket.Cursor()

		// Seek to the secrets prefix
		prefix := []byte("/registry/secrets/")
		for k, _ := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, _ = c.Next() {
			secrets = append(secrets, string(k))
		}

		return nil
	})

	return secrets, err
}

// ListAll lists all keys in the snapshot (for debugging)
func (r *Reader) ListAll() ([]string, error) {
	var keys []string

	err := r.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("key"))
		if bucket == nil {
			return fmt.Errorf("key bucket not found in snapshot")
		}

		c := bucket.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			keys = append(keys, string(k))
		}

		return nil
	})

	return keys, err
}
