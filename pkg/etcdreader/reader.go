package etcdreader

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	bolt "go.etcd.io/bbolt"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/server/v3/mvcc/buckets"
)

// revision represents an MVCC revision
type revision struct {
	main int64
	sub  int64
}

const revBytesLen = 8 + 1 + 8 // main(8) + '_'(1) + sub(8)
const markedRevBytesLen = revBytesLen + 1

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

// Get retrieves a value from etcd by key name (not MVCC revision)
func (r *Reader) Get(key string) ([]byte, error) {
	var data []byte

	err := r.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(buckets.Key.Name())
		if bucket == nil {
			return fmt.Errorf("key bucket not found in snapshot")
		}

		// Iterate through all MVCC entries to find the latest revision for this key
		c := bucket.Cursor()
		var latestRev int64 = -1

		for k, v := c.First(); k != nil; k, v = c.Next() {
			// Decode the MVCC key-value pair
			var kv mvccpb.KeyValue
			if err := kv.Unmarshal(v); err != nil {
				continue // Skip malformed entries
			}

			// Check if this is the key we're looking for
			if string(kv.Key) == key {
				// Check if it's not a tombstone (deleted)
				if !isTombstone(k) {
					// Get the revision
					rev := bytesToRev(k)
					if rev.main > latestRev {
						latestRev = rev.main
						data = make([]byte, len(kv.Value))
						copy(data, kv.Value)
					}
				}
			}
		}

		if latestRev == -1 {
			return fmt.Errorf("key not found: %s", key)
		}

		return nil
	})

	return data, err
}

// ListSecrets lists all secrets in the snapshot
func (r *Reader) ListSecrets() ([]string, error) {
	var secrets []string
	seenKeys := make(map[string]struct{})

	err := r.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(buckets.Key.Name())
		if bucket == nil {
			return fmt.Errorf("key bucket not found in snapshot - this may not be a valid etcd v3 snapshot")
		}

		c := bucket.Cursor()
		// Support both standard Kubernetes and OpenShift secret paths
		prefixes := []string{"/registry/secrets/", "/kubernetes.io/secrets/"}

		for k, v := c.First(); k != nil; k, v = c.Next() {
			// Unmarshal the MVCC KeyValue
			var kv mvccpb.KeyValue
			if err := kv.Unmarshal(v); err != nil {
				continue // Skip malformed entries
			}

			key := string(kv.Key)

			// Check if this key is a secret (match any prefix)
			for _, prefix := range prefixes {
				if strings.HasPrefix(key, prefix) {
					// Handle tombstones (deleted keys)
					if !isTombstone(k) {
						seenKeys[key] = struct{}{}
					} else {
						delete(seenKeys, key)
					}
					break
				}
			}
		}

		// Convert map to slice
		for key := range seenKeys {
			secrets = append(secrets, key)
		}

		return nil
	})

	return secrets, err
}

// ListAll lists all keys in the snapshot (for debugging)
func (r *Reader) ListAll() ([]string, error) {
	var keys []string
	seenKeys := make(map[string]struct{})

	err := r.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(buckets.Key.Name())
		if bucket == nil {
			return fmt.Errorf("key bucket not found in snapshot")
		}

		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			// Unmarshal the MVCC KeyValue
			var kv mvccpb.KeyValue
			if err := kv.Unmarshal(v); err != nil {
				continue
			}

			key := string(kv.Key)

			// Handle tombstones
			if !isTombstone(k) {
				seenKeys[key] = struct{}{}
			} else {
				delete(seenKeys, key)
			}
		}

		// Convert map to slice
		for key := range seenKeys {
			keys = append(keys, key)
		}

		return nil
	})

	return keys, err
}

// bytesToRev converts a byte slice to a revision
// Based on etcd's mvcc encoding format
func bytesToRev(bytes []byte) revision {
	return revision{
		main: int64(binary.BigEndian.Uint64(bytes[0:8])),
		sub:  int64(binary.BigEndian.Uint64(bytes[9:])),
	}
}

// isTombstone checks if the key is marked as deleted (tombstone)
// Tombstones have an extra byte at the end
func isTombstone(b []byte) bool {
	return len(b) == markedRevBytesLen
}

// Helper function to check if a key matches a prefix
func hasPrefix(key string, prefix string) bool {
	return bytes.HasPrefix([]byte(key), []byte(prefix))
}
