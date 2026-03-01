# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.4] - 2026-03-01

### Added

* **TTL (Time-To-Live):** Optional support for key expiration. Added method `PutWithTTL(collection, key, value, ttl)`.
* **Lexicographic Iterators:** New method `NewIterator(prefix)` that allows efficient streaming through keys in alphabetical order.
* **Bloom Filters:** Integration of probabilistic filters to avoid unnecessary disk access for non-existent keys.
* **Index Hinting:** `.hint` file system for instant index loading during startup (O(1)).
* **Automatic Compression:** Support for data compression (Deflate) for values larger than 128 bytes, reducing disk usage.
* **File Format v3:** New record header to support expiration fields and control flags.

### Changed

* **Version Upgrade:** The database now uses file format version 3. Files from previous versions (v1/v2) are not compatible.

### Optimized

* **Smart Compaction:** The `Compact()` method now automatically removes records that have already expired due to TTL.
* **Hint Invalidation:** Compaction now invalidates outdated hint files to ensure integrity of new offsets.

### Fixed

* Fixed filtering logic to respect each record’s expiration time.


## [1.0.3] - 2026-03-01

### Added
- **Prefix/Namespace Support:** Nova organização de chaves no formato `collection:key` (ex: `users:johndoe`).
- **Document Support (JSON):** Adicionados helpers `PutJSON` e `GetJSON` para suporte nativo a structs Go via JSON.
- **Prefix Scanning:** Implementado `ScanPrefix(prefix string)` para varredura sequencial eficiente de registros.
- **Filtragem por Prefixo:** Implementado `FilterPrefix(prefix string, fn func(key string, value []byte) bool)` para filtragem funcional de alto desempenho após descriptografia.
- **Filtragem de Coleção:** Adicionado método `Filter(collection string, fn ...)` para compatibilidade e filtragem rápida por coleção.
- **Exportação de Tipos:** O tipo `Record` agora é público para uso na API.

### Changed
- **Separador Interno:** Alterado o separador de chaves compostas de `|` para `:` para alinhar com a nova filosofia de namespaces.
- **API de Listagem:** O método `List(collection string)` foi atualizado para utilizar o novo separador `:`.
- **Documentação:** Atualização completa do `DOCS.md` com exemplos de documentos JSON e filtragem.

### Optimized
- **Buffer Pooling:** Uso de `sync.Pool` para reutilização de buffers, reduzindo drasticamente a pressão do Garbage Collector (GC) durante scans.
- **IO Sequencial:** Implementação de leitura via `io.SectionReader` e `bufio.Reader`, otimizando a performance em modelos `append-only`.
- **Minimização de Alocações:** Reuso de buffers para AAD (*Additional Authenticated Data*) e descriptografia, evitando cópias desnecessárias de memória.

### Fixed
- Corrigida a consistência de leitura em arquivos `append-only`, garantindo que apenas a versão mais recente de cada chave (incluindo deleções) seja processada no scan.
