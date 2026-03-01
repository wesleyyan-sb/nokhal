package database

import (
	"encoding/binary"
	"hash/crc32"
)

const (
	magicHeader = "NOKHAL"
	version     = 2

	crcSize            = 4
	timestampSize      = 8
	collectionSizeSize = 4
	keySizeSize        = 4
	valueSizeSize      = 4
	opSize             = 1

	recordHeaderSize = crcSize + timestampSize + collectionSizeSize + keySizeSize + valueSizeSize

	// Authentication constants
	authMagic      = "NOKHAL_VALID" // 12 bytes
	authNonceSize  = 12
	authTagSize    = 16
	authTokenSize  = authNonceSize + len(authMagic) + authTagSize // 12 + 12 + 16 = 40 bytes

	fileHeaderSize = len(magicHeader) + 1 + saltSize + authTokenSize
)

const (
	OpPut byte = iota
	OpDelete
)

type Record struct {
	Timestamp  int64
	Collection string
	Key        string
	Value      []byte
	Op         byte
}

type record struct {
	Timestamp  int64
	Collection []byte
	Key        []byte
	Value      []byte
	Nonce      []byte
	Op         byte
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

func decodeRecordHeader(buf []byte) (timestamp int64, collSize, keySize, valSize int) {
	timestamp = int64(binary.BigEndian.Uint64(buf[crcSize:]))
	collSize = int(binary.BigEndian.Uint32(buf[crcSize+8:]))
	keySize = int(binary.BigEndian.Uint32(buf[crcSize+12:]))
	valSize = int(binary.BigEndian.Uint32(buf[crcSize+16:]))
	return
}
