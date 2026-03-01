package database

import (
	"encoding/binary"
	"hash/crc32"
)

const (
	magicHeader = "NOKHAL"
	version     = 4 // Updated to version 4 for Envelope Encryption

	crcSize            = 4
	timestampSize      = 8
	expiresAtSize      = 8 // New: TTL
	flagsSize          = 1 // New: Flags (Compression, etc)
	collectionSizeSize = 4
	keySizeSize        = 4
	valueSizeSize      = 4
	opSize             = 1

	// V3/V4 Record Header Size (Unchanged)
	recordHeaderSize = crcSize + timestampSize + expiresAtSize + flagsSize + collectionSizeSize + keySizeSize + valueSizeSize

	// Authentication constants (Legacy V3)
	authMagic     = "NOKHAL_VALID" // 12 bytes
	authNonceSize = 12
	authTagSize   = 16
	authTokenSize = authNonceSize + len(authMagic) + authTagSize // 12 + 12 + 16 = 40 bytes

	// Envelope Encryption (V4)
	dekSize          = 32
	encryptedDekSize = dekSize + authTagSize // 32 + 16 = 48
	v4HeaderSize     = len(magicHeader) + 1 + saltSize + authNonceSize + encryptedDekSize
	
	// Legacy Header Sizes
	v3HeaderSize = len(magicHeader) + 1 + saltSize + authTokenSize
)

const (
	OpPut byte = iota
	OpDelete
)

const (
	FlagNone       byte = 0
	FlagCompressed byte = 1 << 0 // Bit 0: 1 = Compressed
)

// Public Record struct (Decrypted)
type Record struct {
	Timestamp  int64
	ExpiresAt  int64 // 0 means no expiration
	Collection string
	Key        string
	Value      []byte
	Op         byte
}

// Internal record struct (Encrypted/On-Disk)
type record struct {
	Timestamp  int64
	ExpiresAt  int64 // 0 means no expiration
	Flags      byte
	Collection []byte
	Key        []byte
	Value      []byte
	Nonce      []byte
	Op         byte
}

func (r *record) Encode() ([]byte, int) {
	totalSize := recordHeaderSize + opSize + len(r.Collection) + len(r.Key) + len(r.Nonce) + len(r.Value)
	buf := make([]byte, totalSize)

	// CRC placeholder at 0-3
	offset := crcSize
	binary.BigEndian.PutUint64(buf[offset:], uint64(r.Timestamp))
	offset += timestampSize
	binary.BigEndian.PutUint64(buf[offset:], uint64(r.ExpiresAt))
	offset += expiresAtSize
	buf[offset] = r.Flags
	offset += flagsSize
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(r.Collection)))
	offset += collectionSizeSize
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(r.Key)))
	offset += keySizeSize
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(r.Value)))
	offset += valueSizeSize

	// Data
	buf[offset] = r.Op
	offset++
	copy(buf[offset:], r.Collection)
	offset += len(r.Collection)
	copy(buf[offset:], r.Key)
	offset += len(r.Key)
	copy(buf[offset:], r.Nonce)
	offset += len(r.Nonce)
	copy(buf[offset:], r.Value)

	crc := crc32.ChecksumIEEE(buf[crcSize:])
	binary.BigEndian.PutUint32(buf[0:], crc)

	return buf, totalSize
}

func decodeRecordHeader(buf []byte) (timestamp int64, expiresAt int64, flags byte, collSize, keySize, valSize int) {
	offset := crcSize
	timestamp = int64(binary.BigEndian.Uint64(buf[offset:]))
	offset += timestampSize
	expiresAt = int64(binary.BigEndian.Uint64(buf[offset:]))
	offset += expiresAtSize
	flags = buf[offset]
	offset += flagsSize
	collSize = int(binary.BigEndian.Uint32(buf[offset:]))
	offset += collectionSizeSize
	keySize = int(binary.BigEndian.Uint32(buf[offset:]))
	offset += keySizeSize
	valSize = int(binary.BigEndian.Uint32(buf[offset:]))
	return
}
