// SamuraiKTServer implements the Samurai Key Transparency protocol.
//
// Unlike OptiksServer which stores (user||version) => value in the MPT,
// SamuraiKTServer stores user => SamuraiCommitment with all version history
// managed by in-memory AccountInfo segment trees. The proof has two parts:
// an MPT witness and KZG range proofs.
//
// Reference: KT_Samurai.md in the repository root.
package kt

import (
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/ethereum/go-ethereum/triedb"

	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/hash"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/crypto/polynomial"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/tree"
)

// SamuraiRangeProofJSON is a JSON-serializable representation of a KZG range proof.
type SamuraiRangeProofJSON struct {
	Idx                  int              `json:"idx"`
	Layer                int              `json:"layer"`
	Commitment           []byte           `json:"commitment"`
	Proof                []byte           `json:"proof"`
	BlockRange           *proof.BlockRange `json:"block_range"`
	DependentCommitments []int            `json:"dependent_commitments"`
}

// SamuraiQueryResult is the response returned by Get(user) for the Samurai protocol.
type SamuraiQueryResult struct {
	Value          []byte                  `json:"value"`
	CurrentVersion uint64                  `json:"current_version"`
	MptProof       [][]byte                `json:"mpt_proof"`
	CommitmentHash []byte                  `json:"commitment_hash"`
	SamuraiProofs  []SamuraiRangeProofJSON `json:"samurai_proofs"`
	LeafHashes     [][]byte                `json:"leaf_hashes"`
}

// SamuraiKTServer holds all state for the Samurai Key Transparency protocol.
// See KT_Samurai.md for the struct definition.
type SamuraiKTServer struct {
	mu sync.RWMutex

	updateBuffer          []OptiksKVPair
	rootCommitment        common.Hash
	rootCommitmentIsDirty bool

	mpt    *trie.Trie
	trieDB *triedb.Database

	samuraiAccounts map[string]*tree.AccountInfo
	currentVersions map[string]uint64
	currentKey      map[string][]byte
	leafHashes      map[string][]common.Hash

	precomputedData *config.PrecomputedData
	batchSize       uint64
	keysUpdated     atomic.Uint64
}

// NewSamuraiKTServer initialises a SamuraiKTServer with a blank in-memory MPT
// and precomputed cryptographic data loaded from paramsDir.
func NewSamuraiKTServer(batchSize uint64, paramsDir string) *SamuraiKTServer {
	srs, err := kzg.SetupSRS(tree.SegmentTreeSize)
	if err != nil {
		panic(fmt.Sprintf("failed to setup SRS: %v", err))
	}
	V, weights, weightCommits := kzg.LoadBarycentricData(tree.SegmentTreeSize, srs, paramsDir)
	precomputedData := &config.PrecomputedData{
		V:             V,
		Weights:       weights,
		WeightCommits: weightCommits,
		SRS:           srs,
	}

	db := triedb.NewDatabase(rawdb.NewMemoryDatabase(), nil)
	mpt := trie.NewEmpty(db)

	s := &SamuraiKTServer{
		updateBuffer:          make([]OptiksKVPair, 0),
		rootCommitment:        types.EmptyRootHash,
		rootCommitmentIsDirty: false,
		mpt:                   mpt,
		trieDB:                db,
		samuraiAccounts:       make(map[string]*tree.AccountInfo),
		currentVersions:       make(map[string]uint64),
		currentKey:            make(map[string][]byte),
		leafHashes:            make(map[string][]common.Hash),
		precomputedData:       precomputedData,
		batchSize:             batchSize,
	}
	s.keysUpdated.Store(0)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			count := s.keysUpdated.Load()
			log.Infof("[samurai] keys updated: %d", count)
		}
	}()

	return s
}

// Put buffers a user->key update.
func (s *SamuraiKTServer) Put(user, key []byte) {
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
func (s *SamuraiKTServer) GetCommitment() []byte {
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

// Get returns the current key value, MPT proof, and Samurai KZG range proofs
// for a user.
func (s *SamuraiKTServer) Get(user []byte) (*SamuraiQueryResult, error) {
	s.mu.RLock()
	if !s.rootCommitmentIsDirty {
		result, err := s.getResult(user)
		s.mu.RUnlock()
		return result, err
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rootCommitmentIsDirty {
		s.applyUpdates()
	}
	return s.getResult(user)
}

// getResult builds the SamuraiQueryResult for a user.
// Must be called with s.mu held.
func (s *SamuraiKTServer) getResult(user []byte) (*SamuraiQueryResult, error) {
	userKey := string(user)
	n := s.currentVersions[userKey]

	trieKey := makeSamuraiTrieKey(user)
	mptProof, err := s.proveKey(trieKey)
	if err != nil {
		return nil, fmt.Errorf("MPT proof for user %s: %w", hex.EncodeToString(user), err)
	}

	var commitmentHash []byte
	var samuraiProofs []SamuraiRangeProofJSON
	var leafHashBytes [][]byte

	if n > 0 {
		ai := s.samuraiAccounts[userKey]
		commitmentHash = hash.CommitmentToHash(ai.CurrentLXBatchCommitment[tree.MaxLayer-1]).Bytes()

		samuraiProofs, err = s.generateSamuraiProofs(userKey)
		if err != nil {
			return nil, fmt.Errorf("samurai proofs for user %s: %w", hex.EncodeToString(user), err)
		}

		hashes := s.leafHashes[userKey]
		leafHashBytes = make([][]byte, len(hashes))
		for i, h := range hashes {
			hCopy := h
			leafHashBytes[i] = hCopy.Bytes()
		}
	}

	return &SamuraiQueryResult{
		Value:          s.currentKey[userKey],
		CurrentVersion: n,
		MptProof:       mptProof,
		CommitmentHash: commitmentHash,
		SamuraiProofs:  samuraiProofs,
		LeafHashes:     leafHashBytes,
	}, nil
}

// applyUpdates flushes the updateBuffer into the segment trees and MPT.
// Must be called with s.mu held.
func (s *SamuraiKTServer) applyUpdates() {
	s.rootCommitmentIsDirty = false

	for _, pair := range s.updateBuffer {
		userKey := string(pair.User)

		s.currentKey[userKey] = common.CopyBytes(pair.Key)

		if _, ok := s.currentVersions[userKey]; !ok {
			s.currentVersions[userKey] = 0
		}
		ver := s.currentVersions[userKey]

		if s.samuraiAccounts[userKey] == nil {
			s.samuraiAccounts[userKey] = tree.NewAccountInfo(
				common.BytesToAddress(pair.User),
				s.precomputedData,
			)
		}
		ai := s.samuraiAccounts[userKey]

		leafHash := hash.BytesToHash(pair.Key)
		s.leafHashes[userKey] = append(s.leafHashes[userKey], leafHash)
		ai.AddLeafNode(ver, leafHash)

		s.currentVersions[userKey] = ver + 1

		commitmentHash := hash.CommitmentToHash(ai.CurrentLXBatchCommitment[tree.MaxLayer-1])
		trieKey := makeSamuraiTrieKey(pair.User)
		trieValue, err := rlp.EncodeToBytes(commitmentHash.Bytes())
		if err != nil {
			log.Errorf("RLP encode failed for user %s: %v", hex.EncodeToString(pair.User), err)
			continue
		}
		if err := s.mpt.Update(trieKey, trieValue); err != nil {
			log.Errorf("MPT update failed for user %s: %v", hex.EncodeToString(pair.User), err)
		}
	}

	count := uint64(len(s.updateBuffer))
	s.updateBuffer = s.updateBuffer[:0]
	s.rootCommitment = s.mpt.Hash()
	s.keysUpdated.Add(count)
}

// generateSamuraiProofs generates KZG range proofs covering all versions
// [0, currentVersions[user]-1] from the in-memory AccountInfo.
func (s *SamuraiKTServer) generateSamuraiProofs(userKey string) ([]SamuraiRangeProofJSON, error) {
	n := s.currentVersions[userKey]
	if n == 0 {
		return nil, nil
	}

	ai := s.samuraiAccounts[userKey]
	endVersion := int(n - 1)

	reqCommits := proof.FindCommitmentsCoveringRange(0, endVersion)

	results := make([]SamuraiRangeProofJSON, len(reqCommits))
	for i, rc := range reqCommits {
		layer := rc.Layer
		idx := rc.Idx

		batchTree := ai.CurrentLXBatchTree[layer-1]
		storedCommitment := ai.CurrentLXBatchCommitment[layer-1]

		nodesToInterpolate := proof.FindNodesToInterpolate(rc, true)

		xs1 := make([]int, len(batchTree))
		ys1 := make([]fr.Element, len(batchTree))
		for j, v := range batchTree {
			xs1[j] = j
			ys1[j] = polynomial.HashToFieldElement(v)
		}
		P := polynomial.Interpolate(xs1, ys1, s.precomputedData.V, s.precomputedData.Weights)

		Z := polynomial.VanishingPolynomial(nodesToInterpolate)

		xs := make([]fr.Element, len(nodesToInterpolate))
		ys := make([]fr.Element, len(nodesToInterpolate))
		for j, v := range nodesToInterpolate {
			xs[j] = fr.NewElement(uint64(v))
			ys[j] = polynomial.HashToFieldElement(batchTree[v])
		}

		I := kzg.Interpolate(xs, ys)
		diff := kzg.SubtractPolys(P, I)
		Q := kzg.PolyDiv(diff, Z)
		QCommit, err := gnark_kzg.Commit(Q, s.precomputedData.SRS.Inner.Pk)
		if err != nil {
			return nil, fmt.Errorf("KZG commit failed layer %d idx %d: %w", layer, idx, err)
		}

		commitBytes := storedCommitment.Marshal()
		proofBytes := QCommit.Marshal()

		results[i] = SamuraiRangeProofJSON{
			Idx:                  idx,
			Layer:                layer,
			Commitment:           commitBytes,
			Proof:                proofBytes,
			BlockRange:           rc.BlockRange,
			DependentCommitments: rc.DependentCommitments,
		}
	}

	return results, nil
}

// proveKey generates an MPT proof for the given trie key.
func (s *SamuraiKTServer) proveKey(trieKey []byte) ([][]byte, error) {
	pc := &proofCollector{}
	if err := s.mpt.Prove(trieKey, pc); err != nil {
		return nil, err
	}
	return pc.nodes, nil
}

// makeSamuraiTrieKey builds the MPT key for a user.
// Pads/truncates the user bytes to 32 bytes.
func makeSamuraiTrieKey(user []byte) []byte {
	buf := make([]byte, 32)
	copy(buf, user)
	return buf
}

// Compile-time interface checks (reuse proofCollector from optiks.go).
var _ ethdb.KeyValueWriter = (*proofCollector)(nil)
