package database

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"testing"
)

func BenchmarkPut(b *testing.B) {
	file, err := os.CreateTemp("", "nokhal_bench_put_*.nok")
	if err != nil {
		b.Fatal(err)
	}
	path := file.Name()
	file.Close()
	defer os.Remove(path)

	db, err := Open(path, "bench_pass")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	val := make([]byte, 100)
	io.ReadFull(rand.Reader, val)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i)
		if err := db.Put("col", key, val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGet(b *testing.B) {
	file, err := os.CreateTemp("", "nokhal_bench_get_*.nok")
	if err != nil {
		b.Fatal(err)
	}
	path := file.Name()
	file.Close()
	defer os.Remove(path)

	db, err := Open(path, "bench_pass")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	val := make([]byte, 100)
	io.ReadFull(rand.Reader, val)

	// Pre-fill
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key_%d", i)
		db.Put("col", key, val)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i%1000)
		if _, err := db.Get("col", key); err != nil {
			b.Fatal(err)
		}
	}
}
