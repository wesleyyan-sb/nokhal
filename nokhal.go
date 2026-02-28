package nokhal

import (
	"github.com/wesleyyan-sb/nokhal/internal/database"
)

// DB represents a Nokhal database instance.
type DB struct {
	inner *database.DB
}

// Open opens or creates a new Nokhal database at the specified path.
func Open(path, password string) (*DB, error) {
	db, err := database.Open(path, password)
	if err != nil {
		return nil, err
	}
	return &DB{inner: db}, nil
}

// Put adds a key-value pair to a collection.
func (db *DB) Put(collection, key string, value []byte) error {
	return db.inner.Put(collection, key, value)
}

// Get retrieves a value from a collection by key.
func (db *DB) Get(collection, key string) ([]byte, error) {
	return db.inner.Get(collection, key)
}

// Delete removes a key from a collection.
func (db *DB) Delete(collection, key string) error {
	return db.inner.Delete(collection, key)
}

// Close closes the database.
func (db *DB) Close() error {
	return db.inner.Close()
}

// Compact reclaims space by removing old versions of keys and deleted records.
func (db *DB) Compact() error {
	return db.inner.Compact()
}

// Errors
var (
	ErrNotFound         = database.ErrNotFound
	ErrChecksumMismatch = database.ErrChecksumMismatch
	ErrInvalidFile      = database.ErrInvalidFile
	ErrDecryption       = database.ErrDecryption
)
