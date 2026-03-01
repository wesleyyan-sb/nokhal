package database

import (
	"io"
	"strings"
)

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

	offset := int64(fileHeaderSize)
	fi, err := db.file.Stat()
	if err != nil {
		return err
	}
	fileSize := fi.Size()

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
		} else if rec.Op == OpDelete {
			delete(db.index, key)
		}
		offset += size
	}
	db.offset = offset
	return nil
}
