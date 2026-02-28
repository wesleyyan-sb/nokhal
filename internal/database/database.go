package database

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	magicHeader = "NOKHAL"
	version = 1
	saltSize = 32
	keySize = 32
	nonceSize = 12

	crcSize            = 4
	timestampSize      = 8
	collectionSizeSize = 4
	keySizeSize        = 4
	valueSizeSize      = 4
	opSize             = 1

	recordHeaderSize = crcSize + timestampSize + collectionSizeSize + keySizeSize + valueSizeSize

	fileHeaderSize = len(magicHeader) + 1 + saltSize
)

const (
	OpPut byte = iota
	OpDelete
)

var (
	ErrNotFound         = errors.New("key not found")
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrInvalidFile      = errors.New("invalid file format")
	ErrDecryption       = errors.New("decryption failed")
)

type DB struct {
	mu         sync.RWMutex
	file       *os.File
	offset     int64
	index      map[string]int64 
	path       string
	aead       cipher.AEAD
	salt       []byte
}

type record struct {
	Timestamp  int64
	Collection []byte
	Key        []byte
	Value      []byte 
	Nonce      []byte
	Op         byte
}

func Open(path, password string) (*DB, error) {
	var file *os.File
	var err error
	var salt []byte

	stat, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	
	if os.IsNotExist(err) || stat.Size() == 0 {
		file, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		if err != nil {
			return nil, err
		}

		salt = make([]byte, saltSize)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			file.Close()
			return nil, err
		}

		header := make([]byte, fileHeaderSize)
		copy(header[0:], magicHeader)
		header[len(magicHeader)] = version
		copy(header[len(magicHeader)+1:], salt)

		if _, err := file.Write(header); err != nil {
			file.Close()
			return nil, err
		}
	} else {
		file, err = os.OpenFile(path, os.O_APPEND|os.O_RDWR, 0644)
		if err != nil {
			return nil, err
		}

		header := make([]byte, fileHeaderSize)
		if _, err := io.ReadFull(file, header); err != nil {
			file.Close()
			return nil, err
		}

		if string(header[:len(magicHeader)]) != magicHeader {
			file.Close()
			return nil, ErrInvalidFile
		}
		if header[len(magicHeader)] != version {
			file.Close()
			return nil, fmt.Errorf("unsupported version: %d", header[len(magicHeader)])
		}

		salt = header[len(magicHeader)+1:]
	}

	key := deriveKey(password, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		file.Close()
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		file.Close()
		return nil, err
	}

	db := &DB{
		file:   file,
		index:  make(map[string]int64),
		path:   path,
		aead:   aead,
		salt:   salt,
	}

	if err := db.loadIndexes(); err != nil {
		file.Close()
		return nil, err
	}

	return db, nil
}

func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, keySize)
}

func compositeKey(collection, key string) string {
	return collection + "|" + key
}

func (db *DB) Put(collection, key string, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	aad := []byte(compositeKey(collection, key))
	encryptedValue := db.aead.Seal(nil, nonce, value, aad)

	rec := &record{
		Timestamp:  time.Now().UnixNano(),
		Collection: []byte(collection),
		Key:        []byte(key),
		Value:      encryptedValue,
		Nonce:      nonce,
		Op:         OpPut,
	}

	return db.writeRecord(rec)
}

func (db *DB) Get(collection, key string) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	offset, ok := db.index[compositeKey(collection, key)]
	if !ok {
		return nil, ErrNotFound
	}

	rec, _, err := db.readRecord(offset)
	if err != nil {
		return nil, err
	}

	aad := []byte(compositeKey(collection, key))
	plaintext, err := db.aead.Open(nil, rec.Nonce, rec.Value, aad)
	if err != nil {
		return nil, ErrDecryption
	}

	return plaintext, nil
}

func (db *DB) Delete(collection, key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	idxKey := compositeKey(collection, key)
	if _, ok := db.index[idxKey]; !ok {
		return nil 
	}

	rec := &record{
		Timestamp:  time.Now().UnixNano(),
		Collection: []byte(collection),
		Key:        []byte(key),
		Value:      nil, 
		Nonce:      make([]byte, nonceSize), 
		Op:         OpDelete,
	}

	if err := db.writeRecord(rec); err != nil {
		return err
	}

	delete(db.index, idxKey)
	return nil
}

func (db *DB) writeRecord(r *record) error {
	encoded, size := r.Encode()
	if _, err := db.file.Write(encoded); err != nil {
		return err
	}

	if r.Op == OpPut {
		db.index[compositeKey(string(r.Collection), string(r.Key))] = db.offset
	}

	db.offset += int64(size)
	return nil
}

func (r *record) Encode() ([]byte, int) {
	totalSize := recordHeaderSize + opSize + len(r.Collection) + len(r.Key) + len(r.Nonce) + len(r.Value)
	buf := make([]byte, totalSize)

	binary.BigEndian.PutUint64(buf[crcSize:], uint64(r.Timestamp))
	binary.BigEndian.PutUint32(buf[crcSize+8:], uint32(len(r.Collection)))
	binary.BigEndian.PutUint32(buf[crcSize+12:], uint32(len(r.Key)))
	binary.BigEndian.PutUint32(buf[crcSize+16:], uint32(len(r.Value)))

	dataOffset := recordHeaderSize
	buf[dataOffset] = r.Op
	dataOffset++
	copy(buf[dataOffset:], r.Collection)
	dataOffset += len(r.Collection)
	copy(buf[dataOffset:], r.Key)
	dataOffset += len(r.Key)
	copy(buf[dataOffset:], r.Nonce)
	dataOffset += len(r.Nonce)
	copy(buf[dataOffset:], r.Value)

	crc := crc32.ChecksumIEEE(buf[crcSize:]) 
	binary.BigEndian.PutUint32(buf[0:], crc)

	return buf, totalSize
}

func (db *DB) readRecord(offset int64) (*record, int64, error) {
	headerBuf := make([]byte, recordHeaderSize)
	if _, err := db.file.ReadAt(headerBuf, offset); err != nil {
		return nil, 0, err
	}

	timestamp := int64(binary.BigEndian.Uint64(headerBuf[crcSize:]))
	collSize := int(binary.BigEndian.Uint32(headerBuf[crcSize+8:]))
	keySize := int(binary.BigEndian.Uint32(headerBuf[crcSize+12:]))
	valSize := int(binary.BigEndian.Uint32(headerBuf[crcSize+16:]))

	dataSize := opSize + collSize + keySize + nonceSize + valSize
	totalSize := recordHeaderSize + dataSize

	fullBuf := make([]byte, totalSize)
	if _, err := db.file.ReadAt(fullBuf, offset); err != nil {
		return nil, 0, err
	}

	storedCRC := binary.BigEndian.Uint32(fullBuf[:crcSize])
	calculatedCRC := crc32.ChecksumIEEE(fullBuf[crcSize:])
	if storedCRC != calculatedCRC {
		return nil, 0, ErrChecksumMismatch
	}

	dataOffset := recordHeaderSize
	op := fullBuf[dataOffset]
	dataOffset++
	
	coll := make([]byte, collSize)
	copy(coll, fullBuf[dataOffset:dataOffset+collSize])
	dataOffset += collSize
	
	key := make([]byte, keySize)
	copy(key, fullBuf[dataOffset:dataOffset+keySize])
	dataOffset += keySize

	nonce := make([]byte, nonceSize)
	copy(nonce, fullBuf[dataOffset:dataOffset+nonceSize])
	dataOffset += nonceSize

	val := make([]byte, valSize)
	copy(val, fullBuf[dataOffset:dataOffset+valSize])

	return &record{
		Timestamp:  timestamp,
		Collection: coll,
		Key:        key,
		Value:      val,
		Nonce:      nonce,
		Op:         op,
	}, int64(totalSize), nil
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

func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.file.Close()
}

func (db *DB) Compact() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tempPath := db.path + ".compact"
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempPath) 
	}()

	header := make([]byte, fileHeaderSize)
	copy(header[0:], magicHeader)
	header[len(magicHeader)] = version
	copy(header[len(magicHeader)+1:], db.salt)
	if _, err := tempFile.Write(header); err != nil {
		return err
	}

	newOffset := int64(fileHeaderSize)
	newIndex := make(map[string]int64)

	for keyStr, oldOffset := range db.index {
		rec, _, err := db.readRecord(oldOffset)
		if err != nil {
			continue 
		}

		encoded, size := rec.Encode()
		if _, err := tempFile.Write(encoded); err != nil {
			return err
		}

		newIndex[keyStr] = newOffset
		newOffset += int64(size)
	}

	if err := tempFile.Sync(); err != nil {
		return err
	}
	tempFile.Close()
	db.file.Close()

	if err := os.Rename(tempPath, db.path); err != nil {
		return err
	}

	db.file, err = os.OpenFile(db.path, os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return err
	}

	db.offset = newOffset
	db.index = newIndex

	return nil
}