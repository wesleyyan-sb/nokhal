# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.2.0] - 2026-03-01

### Added
- **Batching & Atomic Writes:** New `Batch` API to group multiple operations into a single disk write and `fsync`. Significant write performance improvement.
- **Envelope Encryption (V4):** Implementation of DEK/KEK architecture. Data is encrypted with a random Data Encryption Key (DEK), which is wrapped by the user's password (KEK).
- **Richer AAD (Anti-Replay):** AES-GCM Additional Authenticated Data now includes the record timestamp, preventing data replay attacks.
- **Secure Erasure:** The `Compact()` method now overwrites old data with random bytes before deletion to prevent forensic data recovery.
- **File Format V4:** Updated file header and record processing to support envelope encryption and enhanced security metadata.

### Changed
- **Version Upgrade:** Database now uses file format version 4. Previous versions (v1/v2/v3) are not compatible.

### Optimized
- **Write Throughput:** Batch commits reduce system call overhead by consolidating multiple logical records into a single physical write.

## [1.1.0] - 2026-03-01

### Added
- **TTL (Time-To-Live):** Optional support for key expiration. Added method `PutWithTTL(collection, key, value, ttl)`.
- **Lexicographic Iterators:** New method `NewIterator(prefix)` that allows efficient streaming through keys in alphabetical order.
- **Bloom Filters:** Integration of probabilistic filters to avoid unnecessary disk access for non-existent keys.
- **Index Hinting:** `.hint` file system for instant index loading during startup (O(1)).
- **Automatic Compression:** Support for data compression (Deflate) for values larger than 128 bytes, reducing disk usage.
- **File Format v3:** New record header to support expiration fields and control flags.

### Changed
- **Version Upgrade:** The database now uses file format version 3. Files from previous versions (v1/v2) are not compatible.

### Optimized
- **Smart Compaction:** The `Compact()` method now automatically removes records that have already expired due to TTL.
- **Hint Invalidation:** Compaction now invalidates outdated hint files to ensure integrity of new offsets.

### Fixed
- Fixed filtering logic to respect each record’s expiration time.

## [1.0.3] - 2026-03-01

### Added
- **Prefix/Namespace Support:** New key organization using the `collection:key` format (e.g., `users:johndoe`).
- **Document Support (JSON):** Added `PutJSON` and `GetJSON` helpers for native support of Go structs via JSON.
- **Prefix Scanning:** Implemented `ScanPrefix(prefix string)` for efficient sequential record scanning.
- **Prefix Filtering:** Implemented `FilterPrefix(prefix string, fn func(key string, value []byte) bool)` for high-performance functional filtering after decryption.
- **Collection Filtering:** Added `Filter(collection string, fn ...)` method for compatibility and fast filtering by collection.
- **Type Export:** The `Record` type is now public for API usage.

### Changed
- **Internal Separator:** Changed the compound key separator from `|` to `:` to align with the new namespace philosophy.
- **Listing API:** The `List(collection string)` method was updated to use the new `:` separator.
- **Documentation:** Complete update of `DOCS.md` with examples of JSON documents and filtering.

### Optimized
- **Buffer Pooling:** Use of `sync.Pool` for buffer reuse, drastically reducing Garbage Collector (GC) pressure during scans.
- **Sequential IO:** Implemented reading via `io.SectionReader` and `bufio.Reader`, improving performance in append-only models.
- **Allocation Minimization:** Reused buffers for AAD (*Additional Authenticated Data*) and decryption, avoiding unnecessary memory copies.

### Fixed
- Fixed read consistency in append-only files, ensuring that only the most recent version of each key (including deletions) is processed during scans.

---
*Nokhal: Simple, Secure, and High-Performance KV Storage.*
