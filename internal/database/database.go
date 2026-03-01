package database

import (
	"bufio"
	"bytes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound         = errors.New("key not found")
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrInvalidFile      = errors.New("invalid file format")
	ErrDecryption       = errors.New("decryption failed")
	ErrInvalidPassword  = errors.New("invalid password")
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 128*1024) // 128KB
	},
}

type DB struct {
	mu     sync.RWMutex
	file   *os.File
	offset int64
	index  map[string]int64
	path   string
	aead   cipher.AEAD
	salt   []byte
}

func Open(path, password string) (*DB, error) {
	var file *os.File
	var err error
	var salt []byte
	var authToken []byte

	stat, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if os.IsNotExist(err) || stat.Size() == 0 {
		file, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		if err != nil {
			return nil, err
		}

		salt, err = generateSalt()
		if err != nil {
			file.Close()
			return nil, err
		}

		key := deriveKey(password, salt)
		aead, err := newCipher(key)
		if err != nil {
			file.Close()
			return nil, err
		}

		authNonce, err := generateNonce()
		if err != nil {
			file.Close()
			return nil, err
		}

		authToken = aead.Seal(nil, authNonce, []byte(authMagic), nil)
		// prepend nonce to ciphertext to store together
		authToken = append(authNonce, authToken...)

		header := make([]byte, fileHeaderSize)
		copy(header[0:], magicHeader)
		header[len(magicHeader)] = version
		copy(header[len(magicHeader)+1:], salt)
		copy(header[len(magicHeader)+1+saltSize:], authToken)

		if _, err := file.Write(header); err != nil {
			file.Close()
			return nil, err
		}

		db := &DB{
			file:   file,
			index:  make(map[string]int64),
			path:   path,
			aead:   aead,
			salt:   salt,
			offset: int64(fileHeaderSize),
		}
		return db, nil

	} else {
		file, err = os.OpenFile(path, os.O_APPEND|os.O_RDWR, 0644)
		if err != nil {
			return nil, err
		}

		// Try to read the V2 header size. If file is V1, this might fail or read partial.
		header := make([]byte, fileHeaderSize)
		n, err := io.ReadFull(file, header)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			file.Close()
			return nil, err
		}

		// Check magic
		if n < len(magicHeader) || string(header[:len(magicHeader)]) != magicHeader {
			file.Close()
			return nil, ErrInvalidFile
		}

		fileVersion := header[len(magicHeader)]
		if fileVersion != version {
			file.Close()
			return nil, fmt.Errorf("unsupported version: %d (expected %d)", fileVersion, version)
		}

		if n < fileHeaderSize {
			file.Close()
			return nil, ErrInvalidFile // Header incomplete for V2
		}

		salt = header[len(magicHeader)+1 : len(magicHeader)+1+saltSize]
		storedAuthToken := header[len(magicHeader)+1+saltSize:]

		key := deriveKey(password, salt)
		aead, err := newCipher(key)
		if err != nil {
			file.Close()
			return nil, err
		}

		// Verify password
		nonce := storedAuthToken[:authNonceSize]
		ciphertext := storedAuthToken[authNonceSize:]
		plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
		if err != nil || string(plaintext) != authMagic {
			file.Close()
			return nil, ErrInvalidPassword
		}

		db := &DB{
			file:  file,
			index: make(map[string]int64),
			path:  path,
			aead:  aead,
			salt:  salt,
		}

		if err := db.loadIndexes(); err != nil {
			file.Close()
			return nil, err
		}

		return db, nil
	}
}

func (db *DB) Put(collection, key string, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	nonce, err := generateNonce()
	if err != nil {
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

func (db *DB) List(collection string) ([]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var keys []string
	prefix := collection + ":"
	for k := range db.index {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, strings.TrimPrefix(k, prefix))
		}
	}
	return keys, nil
}

func (db *DB) ScanPrefix(prefix string) ([]Record, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	limit := db.offset
	results := make(map[string]Record)

	secReader := io.NewSectionReader(db.file, int64(fileHeaderSize), limit-int64(fileHeaderSize))
	bufReader := bufio.NewReaderSize(secReader, 128*1024)

	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)

	aadBuf := make([]byte, 0, 256)
	decBuf := make([]byte, 0, 1024)

	for {
		header := buf[:recordHeaderSize]
		_, err := io.ReadFull(bufReader, header)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		timestamp, collSize, keySize, valSize := decodeRecordHeader(header)

		dataSize := opSize + collSize + keySize + nonceSize + valSize
		totalSize := recordHeaderSize + dataSize

		var dataBuf []byte
		if totalSize > len(buf) {
			dataBuf = make([]byte, totalSize)
			copy(dataBuf, header)
		} else {
			dataBuf = buf[:totalSize]
		}

		_, err = io.ReadFull(bufReader, dataBuf[recordHeaderSize:])
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// Verify CRC
		storedCRC := binary.BigEndian.Uint32(dataBuf[:crcSize])
		calculatedCRC := crc32.ChecksumIEEE(dataBuf[crcSize:])
		if storedCRC != calculatedCRC {
			return nil, ErrChecksumMismatch
		}

		dataOffset := recordHeaderSize
		op := dataBuf[dataOffset]
		dataOffset++

		recColl := dataBuf[dataOffset : dataOffset+collSize]
		dataOffset += collSize

		recKey := dataBuf[dataOffset : dataOffset+keySize]
		dataOffset += keySize

		fullKey := string(recColl) + ":" + string(recKey)
		if !strings.HasPrefix(fullKey, prefix) {
			continue
		}

		if op == OpDelete {
			delete(results, fullKey)
			continue
		}

		nonce := dataBuf[dataOffset : dataOffset+nonceSize]
		dataOffset += nonceSize
		val := dataBuf[dataOffset : dataOffset+valSize]

		// Construct AAD
		aadBuf = aadBuf[:0]
		aadBuf = append(aadBuf, recColl...)
		aadBuf = append(aadBuf, ':')
		aadBuf = append(aadBuf, recKey...)

		// Decrypt
		var errOpen error
		plaintext, errOpen := db.aead.Open(decBuf[:0], nonce, val, aadBuf)
		if errOpen != nil {
			return nil, ErrDecryption
		}
		decBuf = plaintext

		valCopy := make([]byte, len(plaintext))
		copy(valCopy, plaintext)

		results[fullKey] = Record{
			Timestamp:  timestamp,
			Collection: string(recColl),
			Key:        string(recKey),
			Value:      valCopy,
			Op:         op,
		}
	}

	final := make([]Record, 0, len(results))
	for _, v := range results {
		final = append(final, v)
	}

	return final, nil
}

func (db *DB) FilterPrefix(prefix string, fn func(key string, value []byte) bool) ([][]byte, error) {
	// Re-implementing for early exit and efficiency instead of calling ScanPrefix
	db.mu.RLock()
	defer db.mu.RUnlock()

	limit := db.offset
	results := make(map[string][]byte)

	secReader := io.NewSectionReader(db.file, int64(fileHeaderSize), limit-int64(fileHeaderSize))
	bufReader := bufio.NewReaderSize(secReader, 128*1024)

	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)

	aadBuf := make([]byte, 0, 256)
	decBuf := make([]byte, 0, 1024)

	for {
		header := buf[:recordHeaderSize]
		_, err := io.ReadFull(bufReader, header)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		_, collSize, keySize, valSize := decodeRecordHeader(header)

		dataSize := opSize + collSize + keySize + nonceSize + valSize
		totalSize := recordHeaderSize + dataSize

		var dataBuf []byte
		if totalSize > len(buf) {
			dataBuf = make([]byte, totalSize)
			copy(dataBuf, header)
		} else {
			dataBuf = buf[:totalSize]
		}

		_, err = io.ReadFull(bufReader, dataBuf[recordHeaderSize:])
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// Verify CRC
		storedCRC := binary.BigEndian.Uint32(dataBuf[:crcSize])
		calculatedCRC := crc32.ChecksumIEEE(dataBuf[crcSize:])
		if storedCRC != calculatedCRC {
			return nil, ErrChecksumMismatch
		}

		dataOffset := recordHeaderSize
		op := dataBuf[dataOffset]
		dataOffset++

		recColl := dataBuf[dataOffset : dataOffset+collSize]
		dataOffset += collSize

		recKey := dataBuf[dataOffset : dataOffset+keySize]
		dataOffset += keySize

		fullKey := string(recColl) + ":" + string(recKey)
		if !strings.HasPrefix(fullKey, prefix) {
			continue
		}

		if op == OpDelete {
			delete(results, fullKey)
			continue
		}

		nonce := dataBuf[dataOffset : dataOffset+nonceSize]
		dataOffset += nonceSize
		val := dataBuf[dataOffset : dataOffset+valSize]

		// Construct AAD
		aadBuf = aadBuf[:0]
		aadBuf = append(aadBuf, recColl...)
		aadBuf = append(aadBuf, ':')
		aadBuf = append(aadBuf, recKey...)

		// Decrypt
		var errOpen error
		plaintext, errOpen := db.aead.Open(decBuf[:0], nonce, val, aadBuf)
		if errOpen != nil {
			return nil, ErrDecryption
		}
		decBuf = plaintext

		if fn(fullKey, plaintext) {
			valCopy := make([]byte, len(plaintext))
			copy(valCopy, plaintext)
			results[fullKey] = valCopy
		} else {
			delete(results, fullKey)
		}
	}

	final := make([][]byte, 0, len(results))
	for _, v := range results {
		final = append(final, v)
	}

	return final, nil
}

func (db *DB) Filter(collection string, fn func(key string, value []byte) bool) ([][]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	limit := db.offset
	results := make(map[string][]byte)
	collBytes := []byte(collection)

	secReader := io.NewSectionReader(db.file, int64(fileHeaderSize), limit-int64(fileHeaderSize))
	bufReader := bufio.NewReaderSize(secReader, 128*1024)

	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)

	// Reuse buffers for AAD and decryption
	aadBuf := make([]byte, 0, 256)
	decBuf := make([]byte, 0, 1024)

	for {
		header := buf[:recordHeaderSize]
		_, err := io.ReadFull(bufReader, header)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		_, collSize, keySize, valSize := decodeRecordHeader(header)

		dataSize := opSize + collSize + keySize + nonceSize + valSize
		totalSize := recordHeaderSize + dataSize

		var dataBuf []byte
		if totalSize > len(buf) {
			dataBuf = make([]byte, totalSize)
			copy(dataBuf, header)
		} else {
			dataBuf = buf[:totalSize]
		}

		_, err = io.ReadFull(bufReader, dataBuf[recordHeaderSize:])
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// Verify CRC
		storedCRC := binary.BigEndian.Uint32(dataBuf[:crcSize])
		calculatedCRC := crc32.ChecksumIEEE(dataBuf[crcSize:])
		if storedCRC != calculatedCRC {
			return nil, ErrChecksumMismatch
		}

		// Extract fields
		dataOffset := recordHeaderSize
		op := dataBuf[dataOffset]
		dataOffset++

		recColl := dataBuf[dataOffset : dataOffset+collSize]
		dataOffset += collSize

		if !bytes.Equal(recColl, collBytes) {
			continue
		}

		recKey := dataBuf[dataOffset : dataOffset+keySize]
		dataOffset += keySize
		keyStr := string(recKey)

		if op == OpDelete {
			delete(results, keyStr)
			continue
		}

		nonce := dataBuf[dataOffset : dataOffset+nonceSize]
		dataOffset += nonceSize
		val := dataBuf[dataOffset : dataOffset+valSize]

		// Construct AAD
		aadBuf = aadBuf[:0]
		aadBuf = append(aadBuf, recColl...)
		aadBuf = append(aadBuf, ':')
		aadBuf = append(aadBuf, recKey...)

		// Decrypt
		var errOpen error
		plaintext, errOpen := db.aead.Open(decBuf[:0], nonce, val, aadBuf)
		if errOpen != nil {
			return nil, ErrDecryption
		}
		decBuf = plaintext

		// Apply filter
		if fn(keyStr, plaintext) {
			valCopy := make([]byte, len(plaintext))
			copy(valCopy, plaintext)
			results[keyStr] = valCopy
		} else {
			delete(results, keyStr)
		}
	}

	final := make([][]byte, 0, len(results))
	for _, v := range results {
		final = append(final, v)
	}

	return final, nil
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

func (db *DB) readRecord(offset int64) (*record, int64, error) {
	headerBuf := make([]byte, recordHeaderSize)
	if _, err := db.file.ReadAt(headerBuf, offset); err != nil {
		return nil, 0, err
	}

	timestamp, collSize, keySize, valSize := decodeRecordHeader(headerBuf)

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

	// Read original header
	originalHeader := make([]byte, fileHeaderSize)
	if _, err := db.file.ReadAt(originalHeader, 0); err != nil {
		return err
	}

	if _, err := tempFile.Write(originalHeader); err != nil {
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
