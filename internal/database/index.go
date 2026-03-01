package database

import (
	"encoding/binary"
	"encoding/gob"
	"errors"
	"hash/fnv"
	"io"
	"os"
	"strings"
)

const hintMagic = "NOKHAL_HINT"

// BloomFilter is a simple probabilistic data structure
type BloomFilter struct {
	Bitset []bool
	Size   uint
}

func NewBloomFilter(size uint) *BloomFilter {
	return &BloomFilter{
		Bitset: make([]bool, size),
		Size:   size,
	}
}

func (bf *BloomFilter) Add(key string) {
	idx := bf.hash(key) % bf.Size
	bf.Bitset[idx] = true
}

func (bf *BloomFilter) Contains(key string) bool {
	idx := bf.hash(key) % bf.Size
	return bf.Bitset[idx]
}

func (bf *BloomFilter) hash(s string) uint {
	h := fnv.New32a()
	h.Write([]byte(s))
	return uint(h.Sum32())
}

func compositeKey(collection, key string) string {
	return collection + ":" + key
}

func SplitKey(fullKey string) (string, string) {
	parts := strings.SplitN(fullKey, ":", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

func (db *DB) loadIndexes() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Try to load from hint file first
	loadedOffset, err := db.loadHint()
	if err == nil {
		db.offset = loadedOffset
	} else {
		// If hint fails, start from beginning
		db.offset = int64(v4HeaderSize)
		db.index = make(map[string]int64)
		db.bloom = NewBloomFilter(100000)
	}

	offset := db.offset
	fi, err := db.file.Stat()
	if err != nil {
		return err
	}
	fileSize := fi.Size()

	// Scan remaining records (or all if no hint)
	for offset < fileSize {
		rec, size, err := db.readRecord(offset)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		key := compositeKey(string(rec.Collection), string(rec.Key))
		if rec.Op == OpPut {
			db.index[key] = offset
			db.bloom.Add(key)
		} else if rec.Op == OpDelete {
			delete(db.index, key)
			// Cannot remove from Bloom Filter (without counting BF), strictly speaking.
			// But for simplicity we ignore removal from BF. 
			// It just means potential false positives, which is BF nature.
		}
		offset += size
	}
	db.offset = offset
	return nil
}

func (db *DB) saveHint() error {
	hintPath := db.path + ".hint"
	f, err := os.Create(hintPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write Header
	if _, err := f.WriteString(hintMagic); err != nil {
		return err
	}

	// Write Last Offset
	if err := binary.Write(f, binary.BigEndian, db.offset); err != nil {
		return err
	}

	// Encode Index and Bloom Filter
	enc := gob.NewEncoder(f)
	if err := enc.Encode(db.index); err != nil {
		return err
	}
	if err := enc.Encode(db.bloom); err != nil {
		return err
	}

	return nil
}

func (db *DB) loadHint() (int64, error) {
	hintPath := db.path + ".hint"
	f, err := os.Open(hintPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// Verify Header
	magic := make([]byte, len(hintMagic))
	if _, err := io.ReadFull(f, magic); err != nil {
		return 0, err
	}
	if string(magic) != hintMagic {
		return 0, errors.New("invalid hint file")
	}

	// Read Offset
	var offset int64
	if err := binary.Read(f, binary.BigEndian, &offset); err != nil {
		return 0, err
	}

	// Decode Index and Bloom Filter
	dec := gob.NewDecoder(f)
	if err := dec.Decode(&db.index); err != nil {
		return 0, err
	}
	if err := dec.Decode(&db.bloom); err != nil {
		return 0, err
	}

	return offset, nil
}
