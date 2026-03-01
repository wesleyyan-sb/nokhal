# Nokhal Documentation

Nokhal is a lightweight, secure, and easy-to-use key-value storage engine for Go, designed with a focus on data privacy and simplicity. It features built-in AES-256 encryption (GCM), Argon2id key derivation, and a log-structured storage model.

<img src="https://raw.githubusercontent.com/wesleyyan-sb/nokhal/refs/heads/main/nokhal.png" width="300">

## Features

- **Encrypted at Rest:** All data is encrypted using AES-256-GCM.
- **Secure Key Derivation:** Uses Argon2id to derive encryption keys from passwords.
- **Namespace & Prefix Support:** Organize keys using `collection:key` format (e.g., `users:johndoe`).
- **Document Support:** Native JSON marshaling/unmarshaling helpers.
- **Efficient Filtering:** High-performance prefix scanning and functional filtering.
- **Log-Structured Storage:** Efficient writes with a built-in compaction mechanism.
- **Atomic Operations:** Thread-safe operations for concurrent access.
- **Data Integrity:** CRC32 checksums for every record to prevent data corruption.

## Installation

To use Nokhal in your Go project, run:

```bash
go get github.com/wesleyyan-sb/nokhal
```

## Quick Start

Nokhal supports both raw byte slices and structured JSON data:

```go
package main

import (
    "fmt"
    "log"
    "github.com/wesleyyan-sb/nokhal"
)

type User struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

func main() {
    db, err := nokhal.Open("data.nok", "my-secret-password")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // 1. Working with JSON (Document-style)
    user := User{Name: "Alice", Age: 30}
    db.PutJSON("users:alice", user)

    var retrieved User
    if err := db.GetJSON("users:alice", &retrieved); err == nil {
        fmt.Printf("User: %+v\n", retrieved)
    }

    // 2. Working with raw bytes
    db.Put("config", "theme", []byte("dark"))

    // 3. Filtering by prefix and criteria
    // Returns all values in "users:" where Age > 18
    results, _ := db.FilterPrefix("users:", func(key string, value []byte) bool {
        var u User
        json.Unmarshal(value, &u)
        return u.Age > 18
    })
    fmt.Printf("Found %d adults\n", len(results))
}
```

## API Reference

### `Open(path string, password string) (*DB, error)`
Opens or creates a database file. The password is required for all subsequent operations.

### `db.Put(collection string, key string, value []byte) error`
Stores raw bytes. Internally, this combines into `collection:key`.

### `db.PutJSON(fullKey string, v any) error`
Marshals `v` to JSON and stores it. `fullKey` should follow the `collection:key` format.

### `db.Get(collection string, key string) ([]byte, error)`
Retrieves raw bytes for a given collection and key.

### `db.GetJSON(fullKey string, dest any) error`
Retrieves and unmarshals a JSON document into `dest`.

### `db.ScanPrefix(prefix string) ([]Record, error)`
Performs an optimized sequential scan of the database and returns all latest records matching the prefix.

### `db.FilterPrefix(prefix string, fn func(key string, value []byte) bool) ([][]byte, error)`
Scans the database by prefix and applies a filter function to decrypted values. Returns only values where the function returns `true`.

### `db.Delete(collection string, key string) error`
Marks a key as deleted.

### `db.List(collection string) ([]string, error)`
Returns all keys currently present in a specific collection.

### `db.Compact() error`
Reclaims space by removing old versions and deleted records from the append-only log.

### `db.Close() error`
Closes the database file safely.

## Data Structures

### `Record`
Returned by `ScanPrefix`:
```go
type Record struct {
    Timestamp  int64
    Collection string
    Key        string
    Value      []byte
    Op         byte // OpPut or OpDelete
}
```

## Errors

- `nokhal.ErrNotFound`: Key does not exist.
- `nokhal.ErrChecksumMismatch`: Data corruption detected.
- `nokhal.ErrDecryption`: Wrong password or corrupted data.

## Best Practices

1.  **Prefixing:** Use consistent prefixes (e.g., `users:`, `orders:`) to make `ScanPrefix` and `FilterPrefix` efficient.
2.  **JSON Helpers:** Use `PutJSON` and `GetJSON` for structured data to avoid manual boilerplate.
3.  **Compaction:** Run `Compact()` during low-traffic periods to maintain optimal file size and read performance.

## License

Apache 2.0
