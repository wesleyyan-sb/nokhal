package database

import (
	"bufio"
	"bytes"
	"compress/flate"
	"crypto/cipher"
	"crypto/rand"
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

// Compression helpers
func compress(data []byte) ([]byte, error) {
	var b bytes.Buffer
	w, err := flate.NewWriter(&b, flate.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func decompress(data []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	return io.ReadAll(r)
}

func secureDelete(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return os.Remove(path)
	}
	
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return os.Remove(path)
	}
	
	size := info.Size()
	buf := make([]byte, 64*1024)
	if _, err := rand.Read(buf); err != nil {
		f.Close()
		return os.Remove(path)
	}

	for i := int64(0); i < size; i += int64(len(buf)) {
		if _, err := f.Write(buf); err != nil {
			break
		}
	}
	
	f.Sync()
	f.Close()
	return os.Remove(path)
}

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
	aead   cipher.AEAD // Initialized with DEK
	salt   []byte
	bloom  *BloomFilter
}

func Open(path, password string) (*DB, error) {
	var file *os.File
	var err error
	var salt []byte
	var kekNonce []byte
	var encryptedDek []byte
	var dek []byte

	stat, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if os.IsNotExist(err) || stat.Size() == 0 {
		file, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		if err != nil {
			return nil, err
		}

		// 1. Generate Salt
		salt, err = generateSalt()
		if err != nil {
			file.Close()
			return nil, err
		}

		// 2. Derive KEK (Key Encryption Key)
		kek := deriveKey(password, salt)
		kekAead, err := newCipher(kek)
		if err != nil {
			file.Close()
			return nil, err
		}

		// 3. Generate DEK (Data Encryption Key)
		dek = make([]byte, dekSize)
		if _, err := io.ReadFull(rand.Reader, dek); err != nil {
			file.Close()
			return nil, err
		}

		// 4. Encrypt DEK
		kekNonce, err = generateNonce()
		if err != nil {
			file.Close()
			return nil, err
		}
		// AAD for DEK encryption can be empty or static string
		encryptedDek = kekAead.Seal(nil, kekNonce, dek, []byte("NOKHAL_DEK"))

		// 5. Write Header V4
		// Magic(6) + Version(1) + Salt(32) + KEKNonce(12) + EncryptedDEK(48)
		header := make([]byte, v4HeaderSize)
		offset := 0
		copy(header[offset:], magicHeader)
		offset += len(magicHeader)
		header[offset] = version
		offset++
		copy(header[offset:], salt)
		offset += len(salt)
		copy(header[offset:], kekNonce)
		offset += len(kekNonce)
		copy(header[offset:], encryptedDek)

		if _, err := file.Write(header); err != nil {
			file.Close()
			return nil, err
		}

		// 6. Init Data AEAD with DEK
		dataAead, err := newCipher(dek)
		if err != nil {
			file.Close()
			return nil, err
		}

		db := &DB{
			file:   file,
			index:  make(map[string]int64),
			path:   path,
			aead:   dataAead,
			salt:   salt,
			offset: int64(v4HeaderSize),
			bloom:  NewBloomFilter(100000),
		}
		return db, nil

	} else {
		file, err = os.OpenFile(path, os.O_APPEND|os.O_RDWR, 0644)
		if err != nil {
			return nil, err
		}

		// Read V4 Header
		header := make([]byte, v4HeaderSize)
		n, err := io.ReadFull(file, header)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			file.Close()
			return nil, err
		}

		if n < len(magicHeader) || string(header[:len(magicHeader)]) != magicHeader {
			file.Close()
			return nil, ErrInvalidFile
		}

		fileVersion := header[len(magicHeader)]
		if fileVersion != version {
			file.Close()
			return nil, fmt.Errorf("unsupported version: %d (expected %d)", fileVersion, version)
		}

		if n < v4HeaderSize {
			file.Close()
			return nil, ErrInvalidFile
		}

		offset := len(magicHeader) + 1
		salt = header[offset : offset+saltSize]
		offset += saltSize
		kekNonce = header[offset : offset+authNonceSize]
		offset += authNonceSize
		encryptedDek = header[offset : offset+encryptedDekSize]

		// Derive KEK
		kek := deriveKey(password, salt)
		kekAead, err := newCipher(kek)
		if err != nil {
			file.Close()
			return nil, err
		}

		// Decrypt DEK
		dek, err = kekAead.Open(nil, kekNonce, encryptedDek, []byte("NOKHAL_DEK"))
		if err != nil {
			file.Close()
			return nil, ErrInvalidPassword
		}

		// Init Data AEAD
		dataAead, err := newCipher(dek)
		if err != nil {
			file.Close()
			return nil, err
		}

		db := &DB{
			file:  file,
			index: make(map[string]int64),
			path:  path,
			aead:  dataAead,
			salt:  salt,
			bloom: NewBloomFilter(100000),
		}

		if err := db.loadIndexes(); err != nil {
			file.Close()
			return nil, err
		}

		return db, nil
	}
}

func (db *DB) Put(collection, key string, value []byte) error {
	return db.PutWithTTL(collection, key, value, 0)
}

func (db *DB) PutWithTTL(collection, key string, value []byte, ttl time.Duration) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	nonce, err := generateNonce()
	if err != nil {
		return err
	}

	flags := FlagNone
	finalValue := value

	// Compress if larger than 128 bytes
	if len(value) > 128 {
		compressed, err := compress(value)
		if err == nil && len(compressed) < len(value) {
			finalValue = compressed
			flags |= FlagCompressed
		}
	}

	now := time.Now().UnixNano()
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixNano()
	}

	// Richer AAD: Collection:Key + Timestamp
	compKey := compositeKey(collection, key)
	aad := make([]byte, len(compKey)+8)
	copy(aad, compKey)
	binary.BigEndian.PutUint64(aad[len(compKey):], uint64(now))

	encryptedValue := db.aead.Seal(nil, nonce, finalValue, aad)

	rec := &record{
		Timestamp:  now,
		ExpiresAt:  expiresAt,
		Flags:      flags,
		Collection: []byte(collection),
		Key:        []byte(key),
		Value:      encryptedValue,
		Nonce:      nonce,
		Op:         OpPut,
	}

	if err := db.writeRecord(rec); err != nil {
		return err
	}
	
	db.bloom.Add(compKey)
	return nil
}

func (db *DB) Get(collection, key string) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	compKey := compositeKey(collection, key)
	if !db.bloom.Contains(compKey) {
		return nil, ErrNotFound
	}

	offset, ok := db.index[compKey]
	if !ok {
		return nil, ErrNotFound
	}

	rec, _, err := db.readRecord(offset)
	if err != nil {
		return nil, err
	}

	// Check Expiration
	if rec.ExpiresAt > 0 && rec.ExpiresAt < time.Now().UnixNano() {
		return nil, ErrNotFound
	}

	// Reconstruct AAD with stored timestamp
	aad := make([]byte, len(compKey)+8)
	copy(aad, compKey)
	binary.BigEndian.PutUint64(aad[len(compKey):], uint64(rec.Timestamp))

	plaintext, err := db.aead.Open(nil, rec.Nonce, rec.Value, aad)
	if err != nil {
		return nil, ErrDecryption
	}

	// Decompress if needed
	if rec.Flags&FlagCompressed != 0 {
		decompressed, err := decompress(plaintext)
		if err != nil {
			return nil, err
		}
		return decompressed, nil
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

	secReader := io.NewSectionReader(db.file, int64(v4HeaderSize), limit-int64(v4HeaderSize))
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

		timestamp, expiresAt, flags, collSize, keySize, valSize := decodeRecordHeader(header)

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

		// Check Expiration
		if expiresAt > 0 && expiresAt < time.Now().UnixNano() {
			delete(results, fullKey) // Ensure expired key is removed if previously added
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
		// Append Timestamp
		tsBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(tsBuf, uint64(timestamp))
		aadBuf = append(aadBuf, tsBuf...)

		// Decrypt
		var errOpen error
		plaintext, errOpen := db.aead.Open(decBuf[:0], nonce, val, aadBuf)
		if errOpen != nil {
			return nil, ErrDecryption
		}
		decBuf = plaintext

		// Decompress if needed
		finalVal := plaintext
		if flags&FlagCompressed != 0 {
			decompressed, err := decompress(plaintext)
			if err != nil {
				return nil, err
			}
			finalVal = decompressed
		}

		valCopy := make([]byte, len(finalVal))
		copy(valCopy, finalVal)

		results[fullKey] = Record{
			Timestamp:  timestamp,
			ExpiresAt:  expiresAt,
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

	secReader := io.NewSectionReader(db.file, int64(v4HeaderSize), limit-int64(v4HeaderSize))
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

		timestamp, expiresAt, flags, collSize, keySize, valSize := decodeRecordHeader(header)

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

		// Check Expiration
		if expiresAt > 0 && expiresAt < time.Now().UnixNano() {
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
		tsBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(tsBuf, uint64(timestamp))
		aadBuf = append(aadBuf, tsBuf...)

		// Decrypt
		var errOpen error
		plaintext, errOpen := db.aead.Open(decBuf[:0], nonce, val, aadBuf)
		if errOpen != nil {
			return nil, ErrDecryption
		}
		decBuf = plaintext

		// Decompress if needed
		finalVal := plaintext
		if flags&FlagCompressed != 0 {
			decompressed, err := decompress(plaintext)
			if err != nil {
				return nil, err
			}
			finalVal = decompressed
		}

		if fn(fullKey, finalVal) {
			valCopy := make([]byte, len(finalVal))
			copy(valCopy, finalVal)
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

	secReader := io.NewSectionReader(db.file, int64(v4HeaderSize), limit-int64(v4HeaderSize))
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

		timestamp, expiresAt, flags, collSize, keySize, valSize := decodeRecordHeader(header)

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

		// Check Expiration
		if expiresAt > 0 && expiresAt < time.Now().UnixNano() {
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
		tsBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(tsBuf, uint64(timestamp))
		aadBuf = append(aadBuf, tsBuf...)

		// Decrypt
		var errOpen error
		plaintext, errOpen := db.aead.Open(decBuf[:0], nonce, val, aadBuf)
		if errOpen != nil {
			return nil, ErrDecryption
		}
		decBuf = plaintext

		// Decompress if needed
		finalVal := plaintext
		if flags&FlagCompressed != 0 {
			decompressed, err := decompress(plaintext)
			if err != nil {
				return nil, err
			}
			finalVal = decompressed
		}

		// Apply filter
		if fn(keyStr, finalVal) {
			valCopy := make([]byte, len(finalVal))
			copy(valCopy, finalVal)
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
		ExpiresAt:  0,
		Flags:      FlagNone,
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

	timestamp, expiresAt, flags, collSize, keySize, valSize := decodeRecordHeader(headerBuf)

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
		ExpiresAt:  expiresAt,
		Flags:      flags,
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
	_ = db.saveHint()
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

	// Read original header (V4 size)
	originalHeader := make([]byte, v4HeaderSize)
	if _, err := db.file.ReadAt(originalHeader, 0); err != nil {
		return err
	}

	if _, err := tempFile.Write(originalHeader); err != nil {
		return err
	}

	newOffset := int64(v4HeaderSize)
	newIndex := make(map[string]int64)

	now := time.Now().UnixNano()
	for keyStr, oldOffset := range db.index {
		rec, _, err := db.readRecord(oldOffset)
		if err != nil {
			continue
		}

		// Skip expired records during compaction
		if rec.ExpiresAt > 0 && rec.ExpiresAt < now {
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

	// Secure Erase old file before Rename?
	// os.Rename overwrites `db.path`.
	// But `db.path` points to the old data.
	// `Rename` atomic replacement usually deletes the target.
	// To strictly Secure Delete the *old* data, we must first Rename the old data to a temp name, then Secure Delete it?
	// Or explicitly SecureDelete `db.path` before renaming?
	// If we delete `db.path` before rename, there is a moment where file is gone.
	// But `Rename` is atomic.
	// If we want to overwrite the sectors of the *old* file, we must do it before `Rename` replaces it.
	// BUT `Rename` on Windows/Linux replaces the pointer. The old blocks are freed.
	// To secure erase the *old* blocks, we must open `db.path`, overwrite, close, then Rename `tempPath` to `db.path`.
	
	// Secure Erase Logic:
	if err := secureDelete(db.path); err != nil {
		// Log error?
	}

	if err := os.Rename(tempPath, db.path); err != nil {
		return err
	}

	// Remove hint file as offsets have changed
	_ = os.Remove(db.path + ".hint")

	db.file, err = os.OpenFile(db.path, os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return err
	}

	db.offset = newOffset
	db.index = newIndex

	return nil
}
