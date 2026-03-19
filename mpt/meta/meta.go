package meta

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
)

// Key prefixes for metadata stored in ethdb.
var (
	prefixRoot = []byte("meta:root:")
	keyLast    = []byte("meta:last")
	keyStart   = []byte("meta:start")
)

// rootKey returns meta:root:<block BE 8 bytes>.
func rootKey(block uint64) []byte {
	k := make([]byte, len(prefixRoot)+8)
	copy(k, prefixRoot)
	binary.BigEndian.PutUint64(k[len(prefixRoot):], block)
	return k
}

func encodeU64(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func decodeU64(b []byte) (uint64, error) {
	if len(b) != 8 {
		return 0, fmt.Errorf("meta: expected 8 bytes, got %d", len(b))
	}
	return binary.BigEndian.Uint64(b), nil
}

// PutRoot stores block → root mapping.
func PutRoot(db ethdb.KeyValueWriter, block uint64, root common.Hash) error {
	return db.Put(rootKey(block), root.Bytes())
}

// PutRootBatch writes block → root to a batch (never fails).
func PutRootBatch(batch ethdb.KeyValueWriter, block uint64, root common.Hash) {
	_ = batch.Put(rootKey(block), root.Bytes())
}

// GetRoot retrieves root for a given block.
func GetRoot(db ethdb.KeyValueReader, block uint64) (common.Hash, error) {
	val, err := db.Get(rootKey(block))
	if err != nil {
		return common.Hash{}, fmt.Errorf("meta: root not found for block %d: %w", block, err)
	}
	return common.BytesToHash(val), nil
}

// PutLast stores the last processed block number.
func PutLast(db ethdb.KeyValueWriter, block uint64) error {
	return db.Put(keyLast, encodeU64(block))
}

// PutLastBatch writes last block to a batch (never fails).
func PutLastBatch(batch ethdb.KeyValueWriter, block uint64) {
	_ = batch.Put(keyLast, encodeU64(block))
}

// GetLast returns the last processed block number, or (0, error) if not set.
func GetLast(db ethdb.KeyValueReader) (uint64, error) {
	val, err := db.Get(keyLast)
	if err != nil {
		return 0, err
	}
	return decodeU64(val)
}

// HasLast returns true if meta:last is present.
func HasLast(db ethdb.KeyValueReader) bool {
	has, _ := db.Has(keyLast)
	return has
}

// PutStart stores the start block number (written once for sanity).
func PutStart(db ethdb.KeyValueWriter, block uint64) error {
	return db.Put(keyStart, encodeU64(block))
}

// GetStart returns the start block number.
func GetStart(db ethdb.KeyValueReader) (uint64, error) {
	val, err := db.Get(keyStart)
	if err != nil {
		return 0, err
	}
	return decodeU64(val)
}
