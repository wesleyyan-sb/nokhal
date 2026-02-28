package nokhal

import (
	"bytes"
	"os"
	"testing"
)

func TestPublicAPI(t *testing.T) {
	tempFile, err := os.CreateTemp("", "nokhal_public_test_*.nok")
	if err != nil {
		t.Fatal(err)
	}
	path := tempFile.Name()
	tempFile.Close()
	defer os.Remove(path)

	password := "public_pass"
	db, err := Open(path, password)
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	col := "test_col"
	key := "test_key"
	val := []byte("test_val")

	if err := db.Put(col, key, val); err != nil {
		t.Fatalf("Failed to put: %v", err)
	}

	got, err := db.Get(col, key)
	if err != nil {
		t.Fatalf("Failed to get: %v", err)
	}

	if !bytes.Equal(got, val) {
		t.Errorf("Expected %s, got %s", val, got)
	}

	if err := db.Delete(col, key); err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	_, err = db.Get(col, key)
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}
