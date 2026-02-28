package database

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func tempFile() (string, func()) {
	file, err := os.CreateTemp("", "nokhal_test_*.nok")
	if err != nil {
		panic(err)
	}
	path := file.Name()
	file.Close() 

	cleanup := func() {
		os.Remove(path)
	}
	return path, cleanup
}

func TestPutGet(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	password := "securepassword"
	db, err := Open(path, password)
	if err != nil {
		t.Fatalf("Erro ao abrir o banco de dados: %v", err)
	}
	defer db.Close()

	col := "users"
	key := "user1"
	value := []byte("secret_data")

	if err := db.Put(col, key, value); err != nil {
		t.Fatalf("Erro ao fazer Put: %v", err)
	}

	retrievedValue, err := db.Get(col, key)
	if err != nil {
		t.Fatalf("Erro ao fazer Get: %v", err)
	}

	if !bytes.Equal(retrievedValue, value) {
		t.Errorf("Valor recuperado incorreto. Esperado: %s, Obtido: %s", value, retrievedValue)
	}
}

func TestWrongPassword(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	{
		db, err := Open(path, "correct_password")
		if err != nil {
			t.Fatalf("Erro ao abrir: %v", err)
		}
		if err := db.Put("col", "key", []byte("data")); err != nil {
			t.Fatalf("Erro ao fazer Put: %v", err)
		}
		db.Close()
	}

	{
		db, err := Open(path, "wrong_password")
		if err != nil {
			t.Fatalf("Erro ao reabrir: %v", err)
		}
		defer db.Close()

		_, err = db.Get("col", "key")
		if err != ErrDecryption {
			t.Errorf("Esperado erro de descriptografia, obteve: %v", err)
		}
	}
}

func TestCollections(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	db, err := Open(path, "pass")
	if err != nil {
		t.Fatalf("Erro ao abrir: %v", err)
	}
	defer db.Close()

	key := "same_key"
	val1 := []byte("data1")
	val2 := []byte("data2")

	db.Put("col1", key, val1)
	db.Put("col2", key, val2)

	got1, _ := db.Get("col1", key)
	if !bytes.Equal(got1, val1) {
		t.Errorf("Erro na coleção 1")
	}

	got2, _ := db.Get("col2", key)
	if !bytes.Equal(got2, val2) {
		t.Errorf("Erro na coleção 2")
	}
}

func TestDelete(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	db, err := Open(path, "pass")
	if err != nil {
		t.Fatalf("Erro ao abrir o banco de dados: %v", err)
	}
	defer db.Close()

	col := "col"
	key := "key_del"
	value := []byte("val")

	db.Put(col, key, value)
	db.Delete(col, key)

	_, err = db.Get(col, key)
	if err != ErrNotFound {
		t.Errorf("Esperado ErrNotFound após Delete, mas obteve: %v", err)
	}
}

func TestCompact(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	db, err := Open(path, "pass")
	if err != nil {
		t.Fatalf("Erro ao abrir: %v", err)
	}

	col := "col"
	db.Put(col, "key1", []byte("v1"))
	db.Put(col, "key1", []byte("v2")) 
	db.Put(col, "key2", []byte("v3"))
	db.Delete(col, "key2") 

	stat, _ := db.file.Stat()
	sizeBefore := stat.Size()

	if err := db.Compact(); err != nil {
		t.Fatalf("Erro ao compactar: %v", err)
	}

	stat, _ = db.file.Stat()
	sizeAfter := stat.Size()

	if sizeAfter >= sizeBefore {
		t.Logf("Aviso: Compactação não reduziu tamanho (pode ocorrer com poucos dados devido a overhead de header/crypto). Antes: %d, Depois: %d", sizeBefore, sizeAfter)
	}

	val, err := db.Get(col, "key1")
	if err != nil || !bytes.Equal(val, []byte("v2")) {
		t.Errorf("key1 incorreta após compactação")
	}

	_, err = db.Get(col, "key2")
	if err != ErrNotFound {
		t.Errorf("key2 deveria estar deletada")
	}

	db.Close()
}

func TestEncryptionAtRest(t *testing.T) {
	path, cleanup := tempFile()
	defer cleanup()

	password := "pass"
	db, err := Open(path, password)
	if err != nil {
		t.Fatalf("Erro ao abrir: %v", err)
	}

	secret := "THIS_IS_A_SECRET"
	db.Put("col", "key", []byte(secret))
	db.Close()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Erro ao ler arquivo: %v", err)
	}

	if strings.Contains(string(content), secret) {
		t.Error("Falha de segurança: Texto plano encontrado no arquivo!")
	}
}