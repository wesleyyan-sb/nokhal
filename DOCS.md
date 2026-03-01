# Nokhal Documentation

Nokhal is a lightweight, secure, and easy-to-use key-value storage engine for Go, designed with a focus on data privacy and simplicity. It features built-in AES-256 encryption (GCM), Argon2id key derivation, and a log-structured storage model.

## Features

- **Encrypted at Rest:** All data is encrypted using AES-256-GCM.
- **Secure Key Derivation:** Uses Argon2id to derive encryption keys from passwords.
- **Namespace & Prefix Support:** Organize keys using `collection:key` format (e.g., `users:johndoe`).
- **Time-To-Live (TTL):** Optional expiration for each key.
- **Auto-Compression:** Values over 128 bytes are automatically compressed (Deflate).
- **Lexicographical Iteration:** Sorted key traversal with low memory footprint.
- **Performance Optimizations:** Bloom Filters and Index Hinting for near-instant boot and lookups.
- **Data Integrity:** CRC32 checksums for every record.

## Installation

```bash
go get github.com/wesleyyan-sb/nokhal
```

## Quick Start

```go
package main

import (
    "fmt"
    "time"
    "github.com/wesleyyan-sb/nokhal"
)

func main() {
    db, _ := nokhal.Open("data.nok", "password")
    defer db.Close()

    // 1. Put with TTL (expires in 10 minutes)
    db.PutWithTTL("cache", "session_1", []byte("data"), 10*time.Minute)

    // 2. Lexicographical Iterator
    it := db.NewIterator("users:")
    defer it.Close()
    for it.Next() {
        val, _ := it.Value()
        fmt.Printf("Key: %s, Val: %s\n", it.Key(), val)
    }
}
```

## API Reference

### `Open(path string, password string) (*DB, error)`
Opens or creates a database. Performance is O(1) if a `.hint` file exists.

### `db.Put(collection string, key string, value []byte) error`
Stores raw bytes. Combines into `collection:key`.

### `db.PutWithTTL(collection string, key string, value []byte, ttl time.Duration) error`
Stores data with an expiration time. Use `0` for no expiration.

### `db.Get(collection string, key string) ([]byte, error)`
Retrieves bytes. Fast lookup via Bloom Filter + In-memory index. Returns `ErrNotFound` if expired.

### `db.NewIterator(prefix string) *Iterator`
Returns an iterator for keys starting with `prefix`. Iteration is sorted alphabetically.

### `db.ScanPrefix(prefix string) ([]Record, error)`
Returns all matching records at once. For large datasets, use `NewIterator`.

### `db.FilterPrefix(prefix string, fn func(key string, value []byte) bool) ([][]byte, error)`
Functional filtering after decryption and decompression.

### `db.Compact() error`
Reclaims space and removes expired records. Invalidates old `.hint` files.

## Iterator API

- `it.Next() bool`: Advances to the next key.
- `it.Key() string`: Returns the current full key.
- `it.Value() ([]byte, error)`: Decrypts and returns the value.

## Data Structures

### `Record`
```go
type Record struct {
    Timestamp  int64
    ExpiresAt  int64 // 0 if no TTL
    Collection string
    Key        string
    Value      []byte
    Op         byte
}
```

## License

MIT License
