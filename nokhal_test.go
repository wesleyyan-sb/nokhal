package nokhal

import (
	"bytes"
	"encoding/json"
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

func TestFilterPublic(t *testing.T) {
	tempFile, err := os.CreateTemp("", "nokhal_public_filter_*.nok")
	if err != nil {
		t.Fatal(err)
	}
	path := tempFile.Name()
	tempFile.Close()
	defer os.Remove(path)

	db, err := Open(path, "pass")
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	col := "col"
	db.Put(col, "k1", []byte("apple"))
	db.Put(col, "k2", []byte("banana"))
	db.Put(col, "k3", []byte("cherry"))

	results, err := db.Filter(col, func(key string, value []byte) bool {
		return bytes.Contains(value, []byte("a"))
	})

	if err != nil {
		t.Fatalf("Filter failed: %v", err)
	}

	if len(results) != 2 { // apple and banana contain 'a'
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestDocSupport(t *testing.T) {
	tempFile, err := os.CreateTemp("", "nokhal_doc_test_*.nok")
	if err != nil {
		t.Fatal(err)
	}
	path := tempFile.Name()
	tempFile.Close()
	defer os.Remove(path)

	db, err := Open(path, "pass")
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	type User struct {
		Name string
		Age  int
	}

	// Test PutJSON
	user := User{Name: "John", Age: 17}
	if err := db.PutJSON("users:johndoe", user); err != nil {
		t.Fatalf("PutJSON failed: %v", err)
	}

	// Test GetJSON
	var u User
	if err := db.GetJSON("users:johndoe", &u); err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}
	if u.Name != "John" || u.Age != 17 {
		t.Errorf("GetJSON returned wrong data: %+v", u)
	}

	// Add more users
	db.PutJSON("users:alice", User{Name: "Alice", Age: 25})
	db.PutJSON("users:bob", User{Name: "Bob", Age: 30})
	db.PutJSON("other:something", User{Name: "Other", Age: 50})

	// Test ScanPrefix
	records, err := db.ScanPrefix("users:")
	if err != nil {
		t.Fatalf("ScanPrefix failed: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("Expected 3 records for prefix 'users:', got %d", len(records))
	}

	// Test FilterPrefix
	results, err := db.FilterPrefix("users:", func(key string, value []byte) bool {
		var user User
		json.Unmarshal(value, &user)
		return user.Age > 18
	})

	if err != nil {
		t.Fatalf("FilterPrefix failed: %v", err)
	}

	if len(results) != 2 { // Alice (25) and Bob (30)
		t.Errorf("Expected 2 results for FilterPrefix, got %d", len(results))
	}
}
