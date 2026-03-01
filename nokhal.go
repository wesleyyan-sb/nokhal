package nokhal

import (
	"encoding/json"
	"time"

	"github.com/wesleyyan-sb/nokhal/internal/database"
)

// Record represents a decrypted database record.
type Record = database.Record

// Iterator iterates over keys in sorted order.
type Iterator = database.Iterator

// Batch groups multiple operations into a single atomic write.
type Batch struct {
	inner *database.Batch
}

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

// PutWithTTL adds a key-value pair with an expiration time.
func (db *DB) PutWithTTL(collection, key string, value []byte, ttl time.Duration) error {
	return db.inner.PutWithTTL(collection, key, value, ttl)
}

// Get retrieves a value from a collection by key.
func (db *DB) Get(collection, key string) ([]byte, error) {
	return db.inner.Get(collection, key)
}

// List retrieves all keys in a collection.
func (db *DB) List(collection string) ([]string, error) {
	return db.inner.List(collection)
}

// Filter scans a collection and returns only records that satisfy the filter function.
func (db *DB) Filter(collection string, fn func(key string, value []byte) bool) ([][]byte, error) {
	return db.inner.Filter(collection, fn)
}

// ScanPrefix scans the database for records whose combined key (collection:key) starts with prefix.
func (db *DB) ScanPrefix(prefix string) ([]Record, error) {
	return db.inner.ScanPrefix(prefix)
}

// FilterPrefix scans for records by prefix and returns decrypted values that satisfy the filter.
func (db *DB) FilterPrefix(prefix string, fn func(key string, value []byte) bool) ([][]byte, error) {
	return db.inner.FilterPrefix(prefix, fn)
}

// NewIterator creates a new iterator for the given prefix.
func (db *DB) NewIterator(prefix string) *Iterator {
	return db.inner.NewIterator(prefix)
}

// NewBatch creates a new batch operation.
func (db *DB) NewBatch() *Batch {
	return &Batch{inner: db.inner.NewBatch()}
}

// Put adds a put operation to the batch.
func (b *Batch) Put(collection, key string, value []byte, ttl time.Duration) {
	b.inner.Put(collection, key, value, ttl)
}

// Delete adds a delete operation to the batch.
func (b *Batch) Delete(collection, key string) {
	b.inner.Delete(collection, key)
}

// Commit executes all operations in the batch atomically.
func (b *Batch) Commit() error {
	return b.inner.Commit()
}

// PutJSON encodes v as JSON and stores it with the combined key (collection:key).
func (db *DB) PutJSON(fullKey string, v any) error {
	coll, key := database.SplitKey(fullKey)
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return db.inner.Put(coll, key, data)
}

// GetJSON retrieves a value by combined key (collection:key) and decodes it into dest.
func (db *DB) GetJSON(fullKey string, dest any) error {
	coll, key := database.SplitKey(fullKey)
	data, err := db.inner.Get(coll, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
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
	ErrInvalidPassword  = database.ErrInvalidPassword
)
