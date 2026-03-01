# Nokhal Documentation

Nokhal is a lightweight, secure, and easy-to-use key-value storage engine for Go, designed with a focus on data privacy and simplicity. It features built-in AES-256 encryption (GCM), Argon2id key derivation, and a log-structured storage model.

<img src="https://raw.githubusercontent.com/wesleyyan-sb/nokhal/refs/heads/main/nokhal.png" width="300">

## Features

- **Encrypted at Rest (V4):** Advanced Envelope Encryption (DEK/KEK) using AES-256-GCM.
- **Secure Key Derivation:** Uses Argon2id to derive encryption keys from passwords.
- **Namespace & Prefix Support:** Organize keys using `collection:key` format (e.g., `users:johndoe`).
- **Batch Operations:** High-performance atomic writes for multiple records.
- **Time-To-Live (TTL):** Optional expiration for each key.
- **Auto-Compression:** Values over 128 bytes are automatically compressed (Deflate).
- **Lexicographical Iteration:** Sorted key traversal with low memory footprint.
- **Performance Optimizations:** Bloom Filters and Index Hinting for near-instant boot and lookups.
- **Anti-Replay Security:** Timestamp-based AAD for every encrypted record.
- **Secure Erasure:** Foreground data overwriting during compaction to prevent recovery.

## Installation

```bash
go get github.com/wesleyyan-sb/nokhal
```

## Quick Start

### Basic Usage
```go
db, _ := nokhal.Open("data.nok", "password")
defer db.Close()

// Put and Get
db.Put("users", "alice", []byte("data"))
val, _ := db.Get("users", "alice")
```

### Batch Writes
```go
batch := db.NewBatch()
batch.Put("users", "u1", []byte("v1"), 0)
batch.Put("users", "u2", []byte("v2"), 1 * time.Hour)
batch.Delete("users", "u3")
err := batch.Commit() // All operations written in a single disk sync
```

### Iteration
```go
it := db.NewIterator("users:")
defer it.Close()
for it.Next() {
    fmt.Printf("Key: %s, Val: %s\n", it.Key(), it.Value())
}
```

## API Reference

### `Open(path string, password string) (*DB, error)`
Opens or creates a database. Version 4 format includes a 99-byte security header.

### `db.NewBatch() *Batch`
Creates a new batch for atomic, high-performance writes.

### `db.Put(collection string, key string, value []byte) error`
Stores raw bytes. Wrapper for `PutWithTTL` with 0 duration.

### `db.PutWithTTL(collection string, key string, value []byte, ttl time.Duration) error`
Stores data with an expiration time.

### `db.Get(collection string, key string) ([]byte, error)`
Retrieves bytes. Verified against Bloom Filter and AAD Timestamp.

### `db.NewIterator(prefix string) *Iterator`
Returns a lexicographical iterator.

### `db.Compact() error`
Reclaims space, removes expired records, and secure-erases old data.

## Batch API

- `batch.Put(collection, key, value, ttl)`: Adds a put operation to the batch.
- `batch.Delete(collection, key)`: Adds a delete operation to the batch.
- `batch.Commit() error`: Atomically writes and syncs all operations to disk.

## License

Apache 2.0
