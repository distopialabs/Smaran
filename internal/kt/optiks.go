// Package kt implements the Key Transparency protocols (OPTIKS and Samurai).
//
// The OPTIKS protocol maintains a versioned key-value store backed by
// go-ethereum's Merkle Patricia Trie (MPT). Each (user, version) pair maps
// to a leaf in the trie, allowing membership and non-membership proofs.
//
// Reference: KT.md in the repository root.
package kt

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/nepal80m/samurai/internal/logging"
)

var log = logging.GetLogger("kt")

// OptiksKVPair holds a pending user→key update before it is applied to the MPT.
// See KT.md § "Handling Put(user, key)".
type OptiksKVPair struct {
	User []byte
	Key  []byte
}

// OptiksQueryResult is the response returned by Get(user).
// See KT.md § "Handling Get(user)".
type OptiksQueryResult struct {
	// Value is the current key for the user (nil if user has no entries).
	Value []byte `json:"value"`
	// CurrentVersion is the latest version number for the user (0 if absent).
	CurrentVersion uint64 `json:"current_version"`
	// NextVersionNonMembershipProof is the MPT proof for version (n+1),
	// which must always be a non-membership proof.
	NextVersionNonMembershipProof [][]byte `json:"next_version_non_membership_proof"`
	// VersionProofs[j] stores the MPT proof for version (j+1).
	VersionProofs [][][]byte `json:"version_proofs"`
}

// proofCollector implements ethdb.KeyValueWriter to capture the trie proof
// nodes emitted by Trie.Prove. Each call to Put appends the encoded node
// (the value) to the collector.
type proofCollector struct {
	nodes [][]byte
}

func (p *proofCollector) Put(key []byte, value []byte) error {
	p.nodes = append(p.nodes, common.CopyBytes(value))
	return nil
}

func (p *proofCollector) Delete(key []byte) error {
	return nil
}

// Compile-time check that proofCollector satisfies ethdb.KeyValueWriter.
var _ ethdb.KeyValueWriter = (*proofCollector)(nil)

// OptiksServer holds all state for the OPTIKS Key Transparency protocol.
// See KT.md § "OPTIKS protocol" for the struct definition.
type OptiksServer struct {
	mu sync.Mutex

	// updateBuffer accumulates Put calls until the next read forces applyUpdates.
	// See KT.md § "Handling Put(user, key)".
	updateBuffer []OptiksKVPair

	// rootCommitment is the MPT root hash, updated after applyUpdates.
	rootCommitment common.Hash

	// rootCommitmentIsDirty is true when updateBuffer has unapplied entries.
	rootCommitmentIsDirty bool

	// mpt is the in-memory go-ethereum Merkle Patricia Trie.
	// We never call Commit() so the trie remains usable across calls to
	// Hash() and Prove(). See KT.md § "Code structure".
	mpt *trie.Trie

	// trieDB is the backing database for the MPT (in-memory).
	trieDB *triedb.Database

	// currentVersions maps users to their most recent version number.
	// Users not present default to version 0.
	// See KT.md § "OptiksServer" struct definition.
	currentVersions map[string]uint64

	// currentKey maps users to their current key value.
	currentKey map[string][]byte
}

// NewOptiksServer initialises an OptiksServer with a blank in-memory MPT.
// See KT.md: "During gRPC server initialization, initialize OptiksServer
// with a blank MPT."
func NewOptiksServer() *OptiksServer {
	db := triedb.NewDatabase(rawdb.NewMemoryDatabase(), nil)
	mpt := trie.NewEmpty(db)
	return &OptiksServer{
		rootCommitment:        types.EmptyRootHash,
		rootCommitmentIsDirty: false,
		mpt:                   mpt,
		trieDB:                db,
		currentVersions:       make(map[string]uint64),
		currentKey:            make(map[string][]byte),
	}
}

// Put buffers a user→key update.
// See KT.md § "Handling Put(user, key)".
func (s *OptiksServer) Put(user, key []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updateBuffer = append(s.updateBuffer, OptiksKVPair{
		User: common.CopyBytes(user),
		Key:  common.CopyBytes(key),
	})
	s.rootCommitmentIsDirty = true
}

// GetCommitment returns the current MPT root hash, applying pending
// updates first if needed.
// See KT.md § "Handling GetCommitment".
func (s *OptiksServer) GetCommitment() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rootCommitmentIsDirty {
		s.applyUpdates()
	}
	return s.rootCommitment.Bytes()
}

// Get returns the current key value and all version proofs for a user.
// See KT.md § "Handling Get(user)".
func (s *OptiksServer) Get(user []byte) (*OptiksQueryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Apply pending updates so the trie is consistent.
	if s.rootCommitmentIsDirty {
		s.applyUpdates()
	}

	userKey := string(user)
	n := s.currentVersions[userKey] // 0 if not present

	// Non-membership proof for version (n+1). This key should never exist
	// in the trie.  See KT.md § "Handling Get(user)".
	trieKey := makeTrieKey(user, n+1)
	nonMembershipProof, err := s.proveKey(trieKey)
	if err != nil {
		return nil, fmt.Errorf("non-membership proof for version %d: %w", n+1, err)
	}

	// Membership proofs for versions 1..n (both inclusive).
	var versionProofs [][][]byte
	if n > 0 {
		versionProofs = make([][][]byte, n)
		for i := uint64(1); i <= n; i++ {
			tk := makeTrieKey(user, i)
			proof, err := s.proveKey(tk)
			if err != nil {
				return nil, fmt.Errorf("membership proof for version %d: %w", i, err)
			}
			versionProofs[i-1] = proof
		}
	}

	return &OptiksQueryResult{
		Value:                         s.currentKey[userKey],
		CurrentVersion:                n,
		NextVersionNonMembershipProof: nonMembershipProof,
		VersionProofs:                 versionProofs,
	}, nil
}

// applyUpdates flushes the updateBuffer into the MPT.
// Must be called with s.mu held.
// See KT.md § "applyUpdates".
func (s *OptiksServer) applyUpdates() {
	// Step 1: clear the dirty flag immediately.
	s.rootCommitmentIsDirty = false

	type trieUpdate struct {
		account []byte // trie key (Keccak256 of user||version)
		value   []byte // RLP-encoded key bytes
	}

	// Step 2: iterate through each buffered pair.
	updates := make([]trieUpdate, 0, len(s.updateBuffer))
	for _, pair := range s.updateBuffer {
		userKey := string(pair.User)

		// Insert user→key into CurrentKey.
		s.currentKey[userKey] = common.CopyBytes(pair.Key)

		// Increment (or initialise) CurrentVersions[user].
		if _, ok := s.currentVersions[userKey]; !ok {
			s.currentVersions[userKey] = 1
		} else {
			s.currentVersions[userKey]++
		}
		ver := s.currentVersions[userKey]

		// Build the trie key: Keccak256(user || bigEndian(version)).
		// This mimics Ethereum address hashing so the key looks like a valid
		// Ethereum account identifier in the MPT.
		_account := makeTrieKey(pair.User, ver)

		// RLP-encode the key value for Ethereum-compatible leaf encoding.
		_value, err := rlp.EncodeToBytes(pair.Key)
		if err != nil {
			log.Errorf("RLP encode failed for user %s version %d: %v",
				hex.EncodeToString(pair.User), ver, err)
			continue
		}

		updates = append(updates, trieUpdate{account: _account, value: _value})
	}

	// Step 3: clear the buffer.
	s.updateBuffer = s.updateBuffer[:0]

	// Step 4: batch-apply all updates to the MPT.
	for _, u := range updates {
		if err := s.mpt.Update(u.account, u.value); err != nil {
			log.Errorf("MPT update failed: %v", err)
		}
	}

	// Step 5: recompute and store the root commitment.
	// Hash() does NOT commit the trie, so the trie stays usable.
	s.rootCommitment = s.mpt.Hash()
}

// proveKey generates an MPT proof for the given trie key and returns the
// encoded proof nodes.
func (s *OptiksServer) proveKey(trieKey []byte) ([][]byte, error) {
	pc := &proofCollector{}
	if err := s.mpt.Prove(trieKey, pc); err != nil {
		log.Errorf("MPT proof failed for key %s: %v", hex.EncodeToString(trieKey), err)
		return nil, err
	}
	log.Debugf("MPT proof succeeded for key %s %v", hex.EncodeToString(trieKey), pc.nodes)
	return pc.nodes, nil
}

// makeTrieKey builds the trie lookup key for a (user, version) pair.
// It concatenates the user bytes with the big-endian uint64 version,
// then Keccak256-hashes the result (mirroring how Ethereum hashes account
// addresses before trie lookup).
func makeTrieKey(user []byte, version uint64) []byte {
	buf := make([]byte, len(user)+8)
	copy(buf, user)
	binary.BigEndian.PutUint64(buf[len(user):], version)
	return crypto.Keccak256(buf)
}
