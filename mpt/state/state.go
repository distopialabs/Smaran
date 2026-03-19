package state

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/ethdb/pebble"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/ethereum/go-ethereum/triedb/hashdb"
	"github.com/holiman/uint256"
)

// Well-known Ethereum constants.
var (
	// EmptyRootHash is the root of an empty Merkle Patricia Trie.
	EmptyRootHash = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
	// EmptyCodeHash is keccak256 of empty bytes.
	EmptyCodeHash = crypto.Keccak256Hash(nil)
)

// MPTStateStore wraps go-ethereum's ethdb + triedb for state management.
type MPTStateStore struct {
	DiskDB    ethdb.Database
	TrieDB    *triedb.Database
	cachingDB *state.CachingDB
}

// OpenDB opens (or creates) the ethdb database at the given path.
// backend should be "pebble" or "leveldb".
func OpenDB(path string) (*MPTStateStore, error) {
	var kvStore ethdb.KeyValueStore
	var err error

	kvStore, err = pebble.New(path, 512, 512, "", false)
	if err != nil {
		return nil, fmt.Errorf("failed to open pebble db at %s: %w", path, err)
	}

	db := rawdb.NewDatabase(kvStore)
	tdb := triedb.NewDatabase(db, &triedb.Config{
		HashDB: &hashdb.Config{
			CleanCacheSize: 614 * 1024 * 1024, // 614 MB (matches geth default)
		},
	})

	sdb := state.NewDatabase(tdb, nil)

	return &MPTStateStore{DiskDB: db, TrieDB: tdb, cachingDB: sdb}, nil
}

// Close closes the database.
func (s *MPTStateStore) Close() error {
	if s.TrieDB != nil {
		_ = s.TrieDB.Close()
	}
	if s.DiskDB != nil {
		s.DiskDB.Close()
	}
	return nil
}

func (s *MPTStateStore) OpenState(root common.Hash) (*state.StateDB, error) {
	stateDB, err := state.New(root, s.cachingDB)
	if err != nil {
		return nil, fmt.Errorf("failed to open state at root %s: %w", root.Hex(), err)
	}
	return stateDB, nil
}

// func (s *MPTStateStore) OpenState(root common.Hash) (*trie.StateTrie, error) {
// 	tr, err := trie.NewStateTrie(trie.StateTrieID(root), s.TrieDB)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to open state at root %s: %w", root.Hex(), err)
// 	}
// 	return tr, nil
// }

func (s *MPTStateStore) CommitState(stateDB *state.StateDB, parentRoot common.Hash, blockNum uint64) (common.Hash, error) {
	root, err := stateDB.Commit(blockNum, true, true)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to commit state at block %d: %w", blockNum, err)
	}
	return root, nil
}

// func (s *MPTStateStore) CommitState(tr *trie.StateTrie, parentRoot common.Hash, blockNumber uint64) (common.Hash, error) {
// 	root, nodes := tr.Commit(false)

// 	// Register dirty nodes with the triedb so they can be flushed to disk.
// 	if nodes != nil {
// 		if err := s.TrieDB.Update(root, parentRoot, blockNumber, trienode.NewWithNodeSet(nodes), nil); err != nil {
// 			return common.Hash{}, fmt.Errorf("triedb update failed: %w", err)
// 		}
// 	}
// 	return root, nil
// }

func (s *MPTStateStore) FlushTrieDB(root common.Hash) error {
	if err := s.TrieDB.Commit(root, false); err != nil {
		return fmt.Errorf("triedb flush failed: %w", err)
	}
	return nil
}

// SetAccountBalance sets the balance on a StateDB account.
func SetAccountBalance(stateDB *state.StateDB, addr common.Address, bal *big.Int) {
	u256Bal, _ := uint256.FromBig(bal)
	if u256Bal == nil {
		u256Bal = new(uint256.Int)
	}
	stateDB.SetBalance(addr, u256Bal, tracing.BalanceChangeUnspecified)
}

func SetAccountCommitmentAsBalance(stateDB *state.StateDB, addr common.Address, c common.Hash) {
	v := new(uint256.Int)
	v.SetBytes(c.Bytes()) // exact 32-byte value
	stateDB.SetBalance(addr, v, tracing.BalanceChangeUnspecified)
}

// IsAccountEmpty checks if an account is "empty" per EIP-161.
func IsAccountEmpty(stateDB *state.StateDB, addr common.Address) bool {
	return stateDB.Empty(addr)
}
