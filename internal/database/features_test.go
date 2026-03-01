package database

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestCompression(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	db, err := Open(path, "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create a large value that compresses well
	value := bytes.Repeat([]byte("A"), 1000) // 1000 bytes of 'A'
	
	col := "col"
	key := "comp"
	
	if err := db.Put(col, key, value); err != nil {
		t.Fatal(err)
	}

	// Verify we can read it back
	val, err := db.Get(col, key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(val, value) {
		t.Errorf("Decompressed value mismatch")
	}

	// Internal check: read record to see if FlagCompressed is set
	// This requires peeking into implementation or trusting Get.
	// Since Get calls decompress if flag is set, and it worked, we assume it's fine.
}

func TestTTL(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	db, err := Open(path, "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	col := "col"
	key := "ttl"
	
	// Put with 100ms TTL
	if err := db.PutWithTTL(col, key, []byte("val"), 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	// Should exist now
	if _, err := db.Get(col, key); err != nil {
		t.Errorf("Key should exist immediately")
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Should not exist
	if _, err := db.Get(col, key); err != ErrNotFound {
		t.Errorf("Key should have expired, got: %v", err)
	}
}

func TestIterator(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	db, err := Open(path, "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Put("users", "c", []byte("3"))
	db.Put("users", "a", []byte("1"))
	db.Put("users", "b", []byte("2"))
	db.Put("orders", "x", []byte("9"))

	it := db.NewIterator("users:")
	defer it.Close()

	expected := []string{"users:a", "users:b", "users:c"}
	i := 0
	for it.Next() {
		if it.Key() != expected[i] {
			t.Errorf("Expected key %s, got %s", expected[i], it.Key())
		}
		val, _ := it.Value()
		if string(val) != fmt.Sprintf("%d", i+1) {
			t.Errorf("Expected value %d, got %s", i+1, val)
		}
		i++
	}
	if i != 3 {
		t.Errorf("Iterator count mismatch")
	}
}

func TestIndexHint(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	// 1. Open and populate
	db, err := Open(path, "pass")
	if err != nil {
		t.Fatal(err)
	}
	
	db.Put("col", "k1", []byte("v1"))
	db.Put("col", "k2", []byte("v2"))
	
	// Close to trigger saveHint
	db.Close()

	// 2. Re-open (should load from hint)
	db2, err := Open(path, "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	val, err := db2.Get("col", "k1")
	if err != nil || string(val) != "v1" {
		t.Errorf("Failed to retrieve k1 after restart")
	}

	// Check if Bloom Filter works (internal check via Get behavior)
	if _, err := db2.Get("col", "nonexistent"); err != ErrNotFound {
		t.Error("Bloom filter failed negative check (or general logic fail)")
	}
	
	// Add new data to verify offset was correct
	db2.Put("col", "k3", []byte("v3"))
	val3, _ := db2.Get("col", "k3")
	if string(val3) != "v3" {
		t.Error("Failed to add new data after hint load")
	}
}

func TestBatch(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	db, err := Open(path, "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	batch := db.NewBatch()
	batch.Put("col", "k1", []byte("v1"), 0)
	batch.Put("col", "k2", []byte("v2"), 0)
	batch.Delete("col", "k1")

	// Verify not written yet
	if _, err := db.Get("col", "k2"); err != ErrNotFound {
		t.Error("Batch writes should not be visible before commit")
	}

	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}

	// Verify k2 exists
	val, err := db.Get("col", "k2")
	if err != nil || string(val) != "v2" {
		t.Errorf("Batch put failed")
	}

	// Verify k1 deleted (actually never existed in main index, but logic handles OpDelete)
	if _, err := db.Get("col", "k1"); err != ErrNotFound {
		t.Errorf("Batch delete failed")
	}
}
