# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.3] - 2026-03-01

### Added

* **Prefix/Namespace Support:** New key organization using the `collection:key` format (e.g., `users:johndoe`).
* **Document Support (JSON):** Added `PutJSON` and `GetJSON` helpers for native support of Go structs via JSON.
* **Prefix Scanning:** Implemented `ScanPrefix(prefix string)` for efficient sequential record scanning.
* **Prefix Filtering:** Implemented `FilterPrefix(prefix string, fn func(key string, value []byte) bool)` for high-performance functional filtering after decryption.
* **Collection Filtering:** Added `Filter(collection string, fn ...)` method for compatibility and fast filtering by collection.
* **Type Export:** The `Record` type is now public for API usage.

### Changed

* **Internal Separator:** Changed the compound key separator from `|` to `:` to align with the new namespace philosophy.
* **Listing API:** The `List(collection string)` method was updated to use the new `:` separator.
* **Documentation:** Complete update of `DOCS.md` with examples of JSON documents and filtering.

### Optimized

* **Buffer Pooling:** Use of `sync.Pool` for buffer reuse, drastically reducing Garbage Collector (GC) pressure during scans.
* **Sequential IO:** Implemented reading via `io.SectionReader` and `bufio.Reader`, improving performance in append-only models.
* **Allocation Minimization:** Reused buffers for AAD (*Additional Authenticated Data*) and decryption, avoiding unnecessary memory copies.

### Fixed

* Fixed read consistency in append-only files, ensuring that only the most recent version of each key (including deletions) is processed during scans.
