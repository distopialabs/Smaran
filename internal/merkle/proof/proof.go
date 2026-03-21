package proof

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"

	st "github.com/nepal80m/samurai/internal/merkle/state"
)

// AccountProofResult mirrors geth's eth_getProof response for an account.
type AccountProofResult struct {
	Address      common.Address  `json:"address"`
	AccountProof []string        `json:"accountProof"`
	Balance      *hexutil.Big    `json:"balance"`
	CodeHash     common.Hash     `json:"codeHash"`
	Nonce        hexutil.Uint64  `json:"nonce"`
	StorageHash  common.Hash     `json:"storageHash"`
	StorageProof []interface{}   `json:"storageProof"`
}

// GenerateAccountProof generates an eth_getProof-style account proof.
// Returns the proof result and the raw proof nodes (for size measurement).
func GenerateAccountProof(stateDB *state.StateDB, root common.Hash, addr common.Address, stateTrie state.Trie) (*AccountProofResult, [][]byte, error) {
	// Secure trie key: keccak256(address)
	secureKey := crypto.Keccak256(addr.Bytes())

	// Step 1: Prove into a memory DB.
	proofDB := memorydb.New()
	if err := stateTrie.Prove(secureKey, proofDB); err != nil {
		return nil, nil, fmt.Errorf("trie.Prove failed: %w", err)
	}

	// Step 2: Get ordered proof nodes via VerifyProof with logging wrapper.
	orderedNodes, err := getOrderedProofNodes(root, secureKey, proofDB)
	if err != nil {
		return nil, nil, fmt.Errorf("ordering proof nodes: %w", err)
	}

	// Step 3: Build hex strings for JSON.
	proofStrings := make([]string, len(orderedNodes))
	for i, node := range orderedNodes {
		proofStrings[i] = hexutil.Encode(node)
	}

	// Step 4: Get account fields from state.
	bal := stateDB.GetBalance(addr)
	balBig := bal.ToBig()

	result := &AccountProofResult{
		Address:      addr,
		AccountProof: proofStrings,
		Balance:      (*hexutil.Big)(balBig),
		CodeHash:     st.EmptyCodeHash,
		Nonce:        0,
		StorageHash:  st.EmptyRootHash,
		StorageProof: []interface{}{},
	}

	return result, orderedNodes, nil
}

// loggingProofDB wraps a memorydb and logs all reads in order.
type loggingProofDB struct {
	db    ethdb_reader
	nodes [][]byte
}

type ethdb_reader interface {
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

// MarshalJSON marshals the proof result to JSON.
func MarshalJSON(result *AccountProofResult) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

// VerifyAccountProof verifies a proof against a root and returns the decoded account.
// Returns (exists, balance, error).
func VerifyAccountProof(root common.Hash, addr common.Address, proofNodes [][]byte) (bool, *big.Int, error) {
	secureKey := crypto.Keccak256(addr.Bytes())

	// Build proof DB keyed by keccak256(node).
	proofDB := memorydb.New()
	for _, node := range proofNodes {
		key := crypto.Keccak256(node)
		proofDB.Put(key, node)
	}

	val, err := trie.VerifyProof(root, secureKey, proofDB)
	if err != nil {
		return false, nil, fmt.Errorf("proof verification failed: %w", err)
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
