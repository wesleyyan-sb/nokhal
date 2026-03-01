package database

import (
	"sort"
	"strings"
)

type Iterator struct {
	db     *DB
	keys   []string
	idx    int
	valid  bool
	prefix string
}

func (db *DB) NewIterator(prefix string) *Iterator {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var keys []string
	// The prefix logic in DB is collection:key or just prefix?
	// The user passes "prefix" to ScanPrefix, which usually implies "collection:" or "collection:p".
	// Since our keys in index are "collection:key", we just filter by that.
	
	for k := range db.index {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	return &Iterator{
		db:     db,
		keys:   keys,
		idx:    -1, // Start before first element
		valid:  false,
		prefix: prefix,
	}
}

func (it *Iterator) Next() bool {
	it.idx++
	if it.idx >= len(it.keys) {
		it.valid = false
		return false
	}
	it.valid = true
	return true
}

func (it *Iterator) Key() string {
	if !it.valid {
		return ""
	}
	// Return the full key (collection:key) or just key?
	// Usually iterator returns what was stored.
	return it.keys[it.idx]
}

func (it *Iterator) Value() ([]byte, error) {
	if !it.valid {
		return nil, ErrNotFound
	}
	key := it.keys[it.idx]
	
	// We need to use Get logic (decrypt, decompress, check expiry)
	// But Get takes (collection, key). Our key is composite.
	// We can add a GetInternal or manually do it.
	// Since Get calls index lookup, and we already have the key, we assume it exists?
	// But it might be expired.
	
	// Let's reuse Get but we need to split the key.
	coll, k := SplitKey(key)
	val, err := it.db.Get(coll, k)
	if err == ErrNotFound {
		// If expired or deleted concurrently (though we have RLock? No, Iterator doesn't hold lock during iteration)
		// Iterator holds a snapshot of keys, but values are read on demand.
		// If value is deleted/expired, we return nil/empty? Or error?
		// Standard iterators usually skip invalid? But Next() already happened.
		return nil, err
	}
	return val, err
}

func (it *Iterator) Close() {
	it.keys = nil
}
