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
	"sync/atomic"
	"time"

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
	// CurrentVersionEpoch is the epoch at which the current key was written.
	CurrentVersionEpoch uint64 `json:"current_version_epoch"`
	// OldVersions holds all previous key values (versions 1..n-1).
	OldVersions [][]byte `json:"old_versions"`
	// OldVersionEpochs holds the epoch for each old key value.
	OldVersionEpochs []uint64 `json:"old_version_epochs"`
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
	mu sync.RWMutex

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

	// currentEpoch maps users to the epoch at which their current key was written.
	currentEpoch map[string]uint64

	// oldKeys maps users to all their previous key values (versions 1..n-1).
	oldKeys map[string][][]byte

	// oldEpochs maps users to the epochs corresponding to each old key.
	oldEpochs map[string][]uint64

	// batchSize triggers applyUpdates when the updateBuffer reaches this size.
	// A value of 0 disables batch-triggered flushing.
	batchSize uint64

	// keysUpdated counts total keys applied, reset every logging interval.
	keysUpdated atomic.Uint64

	// epoch is incremented (fetch-add) on every applyUpdates call.
	// It is appended to each key value stored in the trie.
	epoch atomic.Uint64
}

// NewOptiksServer initialises an OptiksServer with a blank in-memory MPT.
// See KT.md: "During gRPC server initialization, initialize OptiksServer
// with a blank MPT."
func NewOptiksServer(batchSize uint64) *OptiksServer {
	db := triedb.NewDatabase(rawdb.NewMemoryDatabase(), nil)
	mpt := trie.NewEmpty(db)
	s := &OptiksServer{
		updateBuffer:          make([]OptiksKVPair, 0),
		rootCommitment:        types.EmptyRootHash,
		rootCommitmentIsDirty: false,
		mpt:                   mpt,
		trieDB:                db,
		currentVersions:       make(map[string]uint64),
		currentKey:            make(map[string][]byte),
		currentEpoch:          make(map[string]uint64),
		oldKeys:               make(map[string][][]byte),
		oldEpochs:             make(map[string][]uint64),
		batchSize:             batchSize,
		keysUpdated:           atomic.Uint64{},
	}
	s.keysUpdated.Store(0)
	s.epoch.Store(0)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			count := s.keysUpdated.Load()
			log.Infof("keys updated: %d", count)
		}
	}()

	return s
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

	if s.batchSize > 0 && uint64(len(s.updateBuffer)) >= s.batchSize {
		s.applyUpdates()
	}
}

// GetCommitment returns the current MPT root hash, applying pending
// updates first if needed.
// See KT.md § "Handling GetCommitment".
func (s *OptiksServer) GetCommitment() []byte {
	s.mu.RLock()
	if !s.rootCommitmentIsDirty {
		commitment := s.rootCommitment.Bytes()
		s.mu.RUnlock()
		return commitment
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rootCommitmentIsDirty {
		s.applyUpdates()
	}
	return s.rootCommitment.Bytes()
}

// Get returns the current key value and all version proofs for a user.
// When useCaching is true, OldVersions and OldVersionEpochs are returned
// as empty slices. When false, all previous keys and their epochs are included.
// See KT.md § "Handling Get(user)".
func (s *OptiksServer) Get(user []byte, useCaching bool) (*OptiksQueryResult, error) {
	s.mu.RLock()
	if !s.rootCommitmentIsDirty {
		result, err := s.getResult(user, useCaching)
		s.mu.RUnlock()
		return result, err
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rootCommitmentIsDirty {
		s.applyUpdates()
	}
	return s.getResult(user, useCaching)
}

// getResult builds the OptiksQueryResult for a user.
// Must be called with s.mu held (read or write).
func (s *OptiksServer) getResult(user []byte, useCaching bool) (*OptiksQueryResult, error) {
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

	var oldVersions [][]byte
	var oldVersionEpochs []uint64
	if !useCaching {
		oldVersions = s.oldKeys[userKey]
		oldVersionEpochs = s.oldEpochs[userKey]
	}
	if oldVersions == nil {
		oldVersions = [][]byte{}
	}
	if oldVersionEpochs == nil {
		oldVersionEpochs = []uint64{}
	}

	return &OptiksQueryResult{
		Value:                         s.currentKey[userKey],
		CurrentVersion:                n,
		NextVersionNonMembershipProof: nonMembershipProof,
		VersionProofs:                 versionProofs,
		CurrentVersionEpoch:           s.currentEpoch[userKey],
		OldVersions:                   oldVersions,
		OldVersionEpochs:              oldVersionEpochs,
	}, nil
}

// applyUpdates flushes the updateBuffer into the MPT.
// Must be called with s.mu held.
// See KT.md § "applyUpdates".
func (s *OptiksServer) applyUpdates() {
	// Step 1: clear the dirty flag immediately.
	s.rootCommitmentIsDirty = false

	// Advance the epoch (fetch-add).
	epoch := s.epoch.Add(1)

	type trieUpdate struct {
		account []byte // trie key (Keccak256 of user||version)
		value   []byte // RLP-encoded (epoch || key) bytes
	}

	// Step 2: iterate through each buffered pair.
	updates := make([]trieUpdate, 0, len(s.updateBuffer))
	for _, pair := range s.updateBuffer {
		userKey := string(pair.User)

		// Archive the previous key+epoch before overwriting (version >= 2).
		if oldKey, ok := s.currentKey[userKey]; ok {
			s.oldKeys[userKey] = append(s.oldKeys[userKey], common.CopyBytes(oldKey))
			s.oldEpochs[userKey] = append(s.oldEpochs[userKey], s.currentEpoch[userKey])
		}

		// Insert user→key into CurrentKey and record the epoch.
		s.currentKey[userKey] = common.CopyBytes(pair.Key)
		s.currentEpoch[userKey] = epoch

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

		// Build the leaf value: pair.Key || bigEndian(epoch).
		leafValue := make([]byte, 8+len(pair.Key))
		binary.BigEndian.PutUint64(leafValue[:8], epoch) // Order is important here.
		copy(leafValue[8:], pair.Key)

		// RLP-encode the leaf value for Ethereum-compatible leaf encoding.
		_value, err := rlp.EncodeToBytes(leafValue)
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

	s.keysUpdated.Add(uint64(len(updates)))
}

// proveKey generates an MPT proof for the given trie key and returns the
// encoded proof nodes.
func (s *OptiksServer) proveKey(trieKey []byte) ([][]byte, error) {
	pc := &proofCollector{}
	if err := s.mpt.Prove(trieKey, pc); err != nil {
		log.Errorf("MPT proof failed for key %s: %v", hex.EncodeToString(trieKey), err)
		return nil, err
	}
	// log.Debugf("MPT proof succeeded for key %s %v", hex.EncodeToString(trieKey), pc.nodes)
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
