package database

import (
	"encoding/binary"
	"sync"
	"time"
)

type Batch struct {
	db      *DB
	writes  []batchRecord
	mu      sync.Mutex
}

type batchRecord struct {
	collection string
	key        string
	value      []byte
	ttl        time.Duration
	op         byte
}

func (db *DB) NewBatch() *Batch {
	return &Batch{
		db: db,
	}
}

func (b *Batch) Put(collection, key string, value []byte, ttl time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writes = append(b.writes, batchRecord{
		collection: collection,
		key:        key,
		value:      value,
		ttl:        ttl,
		op:         OpPut,
	})
}

func (b *Batch) Delete(collection, key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writes = append(b.writes, batchRecord{
		collection: collection,
		key:        key,
		op:         OpDelete,
	})
}

func (b *Batch) Commit() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.writes) == 0 {
		return nil
	}

	b.db.mu.Lock()
	defer b.db.mu.Unlock()

	// 1. Prepare buffers
	now := time.Now().UnixNano()
	
	// We can write sequentially without calling db.writeRecord repeatedly?
	// db.writeRecord writes to file and updates index.
	// To optimize syscalls, we should buffer all writes into a single buffer and Write once.
	// Then sync once.
	
	var batchBuffer []byte
	
	// We need to store offsets to update index later
	type indexUpdate struct {
		key    string
		offset int64
		op     byte
	}
	var updates []indexUpdate
	startOffset := b.db.offset

	for _, w := range b.writes {
		nonce, err := generateNonce()
		if err != nil {
			return err
		}

		// Prepare Record
		var expiresAt int64
		if w.ttl > 0 {
			expiresAt = time.Now().Add(w.ttl).UnixNano()
		}

		flags := FlagNone
		finalValue := w.value

		// Compression logic
		if w.op == OpPut && len(w.value) > 128 {
			compressed, err := compress(w.value)
			if err == nil && len(compressed) < len(w.value) {
				finalValue = compressed
				flags |= FlagCompressed
			}
		}

		var encryptedValue []byte
		if w.op == OpPut {
			// Richer AAD
			compKey := compositeKey(w.collection, w.key)
			aad := make([]byte, len(compKey)+8)
			copy(aad, compKey)
			binary.BigEndian.PutUint64(aad[len(compKey):], uint64(now))

			encryptedValue = b.db.aead.Seal(nil, nonce, finalValue, aad)
		}

		rec := &record{
			Timestamp:  now,
			ExpiresAt:  expiresAt,
			Flags:      flags,
			Collection: []byte(w.collection),
			Key:        []byte(w.key),
			Value:      encryptedValue,
			Nonce:      nonce,
			Op:         w.op,
		}

		encoded, size := rec.Encode()
		batchBuffer = append(batchBuffer, encoded...)

		// Track index update
		compKey := compositeKey(w.collection, w.key)
		updates = append(updates, indexUpdate{
			key:    compKey,
			offset: startOffset,
			op:     w.op,
		})
		startOffset += int64(size)
	}

	// 2. Single Write
	if _, err := b.db.file.Write(batchBuffer); err != nil {
		return err
	}

	// 3. Single Sync
	if err := b.db.file.Sync(); err != nil {
		return err
	}

	// 4. Update In-Memory Index & Bloom Filter
	for _, u := range updates {
		if u.op == OpPut {
			b.db.index[u.key] = u.offset
			b.db.bloom.Add(u.key)
		} else if u.op == OpDelete {
			delete(b.db.index, u.key)
		}
	}

	// 5. Update Offset
	b.db.offset = startOffset

	// Clear batch
	b.writes = nil
	return nil
}
