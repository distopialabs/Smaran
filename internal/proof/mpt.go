package proof

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"

	"github.com/nepal80m/samurai/internal/crypto/hash"
	"github.com/nepal80m/samurai/internal/tree"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
)

// GenerateMPTProof generates an MPT membership proof for the given account
// at the latest processed block. It returns the block number, the ordered
// proof nodes, and any error.
func GenerateMPTProof(mptStore *st.MPTStateStore, account common.Address) (uint64, [][]byte, error) {
	// Get the latest processed block number from MPT metadata.
	lastBlock, err := meta.GetLast(mptStore.DiskDB)
	if err != nil {
		return 0, nil, fmt.Errorf("get last block from MPT: %w", err)
	}

	// Get the state root for that block.
	stateRoot, err := meta.GetRoot(mptStore.DiskDB, lastBlock)
	if err != nil {
		return 0, nil, fmt.Errorf("get MPT root for block %d: %w", lastBlock, err)
	}

	// Open the account trie directly (state.StateDB lazily initializes
	// its trie, so GetTrie() returns nil before any mutation).
	stateTrie, err := mptStore.OpenTrie(stateRoot)
	if err != nil {
		return 0, nil, fmt.Errorf("open trie at root %s: %w", stateRoot.Hex(), err)
	}

	// Generate the proof.
	secureKey := crypto.Keccak256(account.Bytes())
	proofDB := memorydb.New()
	if err := stateTrie.Prove(secureKey, proofDB); err != nil {
		return 0, nil, fmt.Errorf("trie.Prove failed: %w", err)
	}

	// Get ordered proof nodes via VerifyProof with logging wrapper.
	orderedNodes, err := getOrderedProofNodes(stateRoot, secureKey, proofDB)
	if err != nil {
		return 0, nil, fmt.Errorf("ordering proof nodes: %w", err)
	}

	return lastBlock, orderedNodes, nil
}

// VerifyMPTProof verifies an MPT membership proof for the given account
// against a state root. Returns whether the account exists and the balance
// value stored in the MPT (which is the final commitment hash encoded as uint256).
func VerifyMPTProof(stateRoot common.Hash, account common.Address, proofNodes [][]byte) (bool, *big.Int, error) {
	secureKey := crypto.Keccak256(account.Bytes())

	// Build proof DB keyed by keccak256(node).
	proofDB := memorydb.New()
	for _, node := range proofNodes {
		key := crypto.Keccak256(node)
		proofDB.Put(key, node)
	}

	val, err := trie.VerifyProof(stateRoot, secureKey, proofDB)
	if err != nil {
		return false, nil, fmt.Errorf("MPT proof verification failed: %w", err)
	}

	if val == nil {
		// Account does not exist — valid proof of non-existence.
		return false, big.NewInt(0), nil
	}

	// Decode RLP: [nonce, balance, storageRoot, codeHash]
	var acct struct {
		Nonce       uint64
		Balance     *big.Int
		StorageRoot common.Hash
		CodeHash    []byte
	}
	if err := rlp.DecodeBytes(val, &acct); err != nil {
		return false, nil, fmt.Errorf("RLP decode account: %w", err)
	}

	return true, acct.Balance, nil
}

// VerifyFinalCommitmentHash checks that the MPT-stored balance (final commitment hash)
// matches the expected value computed from the current balance and the top-layer
// batch commitment hash.
//
// finalCommitmentHash = hash(currentBalance.Hash(), topLayerBatchCommitmentHash)
//
// This is the same computation as AccountInfo.CalculateFinalCommitment().
func VerifyFinalCommitmentHash(mptBalance *big.Int, currentBalance *tree.CurrentBalance, topLayerCommitmentHash common.Hash) bool {
	// Compute the expected final commitment hash.
	cbHash := currentBalance.Hash()
	expectedHash := hash.BytesToHash(cbHash.Bytes(), topLayerCommitmentHash.Bytes())

	// The MPT balance is the final commitment hash stored as uint256.
	mptHash := common.BigToHash(mptBalance)

	return expectedHash == mptHash
}

// loggingProofDB wraps a memorydb and logs all reads in traversal order.
type loggingProofDB struct {
	db    ethdbReader
	nodes [][]byte
}

type ethdbReader interface {
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
}

func (l *loggingProofDB) Get(key []byte) ([]byte, error) {
	val, err := l.db.Get(key)
	if err != nil {
		return nil, err
	}
	l.nodes = append(l.nodes, common.CopyBytes(val))
	return val, nil
}

func (l *loggingProofDB) Has(key []byte) (bool, error) {
	return l.db.Has(key)
}

// getOrderedProofNodes uses VerifyProof to get proof nodes in traversal order.
func getOrderedProofNodes(root common.Hash, key []byte, proofDB *memorydb.Database) ([][]byte, error) {
	logger := &loggingProofDB{db: proofDB}
	_, err := trie.VerifyProof(root, key, logger)
	if err != nil {
		return nil, err
	}
	return logger.nodes, nil
}

// GenerateMPTProofAtBlock generates an MPT membership proof for the given account
// at a specific block number. This variant is useful for testing or when the block
// number is known.
func GenerateMPTProofAtBlock(mptStore *st.MPTStateStore, blockNumber uint64, account common.Address) ([][]byte, error) {
	stateRoot, err := meta.GetRoot(mptStore.DiskDB, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("get MPT root for block %d: %w", blockNumber, err)
	}

	stateTrie, err := mptStore.OpenTrie(stateRoot)
	if err != nil {
		return nil, fmt.Errorf("open trie at root %s: %w", stateRoot.Hex(), err)
	}

	secureKey := crypto.Keccak256(account.Bytes())
	proofDB := memorydb.New()
	if err := stateTrie.Prove(secureKey, proofDB); err != nil {
		return nil, fmt.Errorf("trie.Prove failed: %w", err)
	}

	orderedNodes, err := getOrderedProofNodes(stateRoot, secureKey, proofDB)
	if err != nil {
		return nil, fmt.Errorf("ordering proof nodes: %w", err)
	}

	return orderedNodes, nil
}

// GetTrie is a helper interface to access the state trie from stateDB.
// go-ethereum's stateDB exposes GetTrie() which returns a state.Trie interface.
var _ state.Trie = (*trie.StateTrie)(nil) // compile-time check
