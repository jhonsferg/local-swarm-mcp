// Package store implements the scratch key-value store Claude uses to stash
// large intermediate blobs outside conversation context, backed by a
// single-file embedded bbolt database (pure Go, no cgo).
package store

import (
	"fmt"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

var bucketName = []byte("scratch")

// Store is a simple persistent key-value scratch space.
type Store struct {
	db *bolt.DB
}

// Open opens (creating if needed) a bbolt database at path.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create store directory: %w", err)
		}
	}

	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init scratch bucket: %w", err)
	}

	return &Store{db: db}, nil
}

// Close releases the underlying database file.
func (s *Store) Close() error {
	return s.db.Close()
}

// Set stores value under key, overwriting any existing value.
func (s *Store) Set(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).Put([]byte(key), []byte(value))
	})
}

// Get returns the value stored under key, and whether it was found.
func (s *Store) Get(key string) (string, bool, error) {
	var value []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketName).Get([]byte(key))
		if v != nil {
			value = append([]byte(nil), v...)
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	if value == nil {
		return "", false, nil
	}
	return string(value), true, nil
}

// Delete removes key, if present.
func (s *Store) Delete(key string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).Delete([]byte(key))
	})
}

// List returns all keys currently stored.
func (s *Store) List() ([]string, error) {
	var keys []string
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).ForEach(func(k, _ []byte) error {
			keys = append(keys, string(k))
			return nil
		})
	})
	return keys, err
}
