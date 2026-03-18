// SamuraiKTServer implements the Samurai Key Transparency protocol.
//
// Unlike OptiksServer which stores (user||version) => value in the MPT,
// SamuraiKTServer stores user => SamuraiCommitment with all version history
// managed via storage.CreateOrUpdateAccountInfo backed by an in-memory SamuraiDB.
// Proof generation and verification reuse the existing APIs from
// internal/proof (GetNewProofRange / VerifyNewRangeProofs).
package kt

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

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
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/storage"
	"github.com/nepal80m/samurai/internal/tree"
)

// SamuraiRangeProofJSON is a JSON-serializable representation of a KZG range proof.
type SamuraiRangeProofJSON struct {
	Idx                  int               `json:"idx"`
	Layer                int               `json:"layer"`
	Commitment           []byte            `json:"commitment"`
	Proof                []byte            `json:"proof"`
	BlockRange           *proof.BlockRange `json:"block_range"`
	DependentCommitments []int             `json:"dependent_commitments"`
}

// SamuraiQueryResult is the response returned by Get(user) for the Samurai protocol.
type SamuraiQueryResult struct {
	Value              []byte                  `json:"value"`
	CurrentVersion     uint64                  `json:"current_version"`
	MptProof           [][]byte                `json:"mpt_proof"`
	CommitmentHash     []byte                  `json:"commitment_hash"`
	SamuraiProofs      []SamuraiRangeProofJSON `json:"samurai_proofs"`
	HistoricalBalances [][]byte                `json:"historical_balances"`
}

// SamuraiKTServer holds all state for the Samurai Key Transparency protocol.
type SamuraiKTServer struct {
	mu sync.RWMutex

	updateBuffer          []OptiksKVPair
	rootCommitment        common.Hash
	rootCommitmentIsDirty bool

	mpt    *trie.Trie
	trieDB *triedb.Database

	samuraiDB  *db.SamuraiDB
	cache      *storage.Cache
	putCounts  map[string]uint64
	currentKey map[string][]byte

	precomputedData *config.PrecomputedData
	batchSize       uint64
	keysUpdated     atomic.Uint64
}

func newInMemorySamuraiDB() *db.SamuraiDB {
	stateDB, err := db.NewInMemoryPebbleDB()
	if err != nil {
		panic(fmt.Sprintf("failed to create in-memory StateDB: %v", err))
	}
	treeDB, err := db.NewInMemoryPebbleDB()
	if err != nil {
		panic(fmt.Sprintf("failed to create in-memory TreeDB: %v", err))
	}
	historyDB, err := db.NewInMemoryPebbleDB()
	if err != nil {
		panic(fmt.Sprintf("failed to create in-memory HistoryDB: %v", err))
	}
	return &db.SamuraiDB{StateDB: stateDB, TreeDB: treeDB, HistoryDB: historyDB}
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

	mptDB := triedb.NewDatabase(rawdb.NewMemoryDatabase(), nil)
	mpt := trie.NewEmpty(mptDB)

	samuraiDB := newInMemorySamuraiDB()

	cacheCfg := &config.Cache{Size: 1}
	cache, err := storage.NewCache(samuraiDB, cacheCfg, precomputedData)
	if err != nil {
		panic(fmt.Sprintf("failed to create storage cache: %v", err))
	}

	s := &SamuraiKTServer{
		updateBuffer:          make([]OptiksKVPair, 0),
		rootCommitment:        types.EmptyRootHash,
		rootCommitmentIsDirty: false,
		mpt:                   mpt,
		trieDB:                mptDB,
		samuraiDB:             samuraiDB,
		cache:                 cache,
		putCounts:             make(map[string]uint64),
		currentKey:            make(map[string][]byte),
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

func userToAddress(user []byte) common.Address {
	return common.BytesToAddress(user)
}

// getResult builds the SamuraiQueryResult for a user.
func (s *SamuraiKTServer) getResult(user []byte) (*SamuraiQueryResult, error) {
	userKey := string(user)
	n := s.putCounts[userKey]

	trieKey := makeSamuraiTrieKey(user)
	mptProof, err := s.proveKey(trieKey)
	if err != nil {
		return nil, fmt.Errorf("MPT proof for user %s: %w", hex.EncodeToString(user), err)
	}

	var commitmentHash []byte
	var samuraiProofs []SamuraiRangeProofJSON
	var hbBytes [][]byte

	if n > 0 {
		account := userToAddress(user)
		log.Infof("account: %s", account.Hex())
		s.cache.Update(account,
			func(account common.Address) *tree.AccountInfo { return nil },
			func(account common.Address, sdb *db.SamuraiDB) *tree.AccountInfo {
				cbInfo, err := tree.GetCurrentBalanceInfo(account, sdb.StateDB)
				if err != nil {
					if err != db.ErrNotFound {
						panic(err)
					}
					return nil
				}
				batchTree := tree.GetCurrentLXBatchTree(account, sdb.TreeDB)
				batchCommitments := tree.GetLXBatchCommitments(account, cbInfo.Version, sdb.StateDB)
				treeCounts := tree.GetTreeCounts(account, sdb.TreeDB)

				// Fresh LXLeafNodes
				var lxLeafNodes [tree.MaxLayer]map[tree.LeafNodeIdx]common.Hash
				for layer := uint64(1); layer <= tree.MaxLayer; layer++ {
					lxLeafNodes[layer-1] = make(map[tree.LeafNodeIdx]common.Hash)
				}
				for layer := uint64(1); layer <= tree.MaxLayer; layer++ {
					for treeIdx := uint64(0); treeIdx < treeCounts[layer-1]; treeIdx++ {
						for leafIdx := uint64(0); leafIdx < tree.L1BatchSize; leafIdx++ {
							lxLeafNodes[layer-1][tree.LeafNodeIdx{TreeIdx: treeIdx, LeafIdx: leafIdx}] = common.Hash{}
						}
					}
				}
				return &tree.AccountInfo{
					Account:                  account,
					CurrentBalanceInfo:       cbInfo,
					CurrentLXBatchTree:       batchTree,
					CurrentLXBatchCommitment: batchCommitments,
					PrecomputedData:          s.precomputedData,
					DirtyChunks:              tree.InitDirtyChunks(),
					CurrentLXTreeCounts:      treeCounts,
					LXLeafNodes:              lxLeafNodes,
				}
			},
			func(accountInfo *tree.AccountInfo, sdb *db.SamuraiDB) {},
		)
		ai, ok := s.cache.C.Get(account)
		if !ok {
			return nil, fmt.Errorf("account not found in cache for user %s", hex.EncodeToString(user))
		}

		commitmentHash = hash.CommitmentToHash(ai.CurrentLXBatchCommitment[tree.MaxLayer-1]).Bytes()

		version := s.putCounts[string(user)]
		log.Infof("version: %d", version)
		if version > 0 {
			log.Infof("getting new proof range for user %s", account.Hex())
			rangeProofs, historicalBalances := proof.GetNewProofRange(
				account, 0, version-1, s.precomputedData, s.samuraiDB,
			)

			samuraiProofs = make([]SamuraiRangeProofJSON, len(rangeProofs))
			for i, rp := range rangeProofs {
				samuraiProofs[i] = SamuraiRangeProofJSON{
					Idx:                  rp.Idx,
					Layer:                rp.Layer,
					Commitment:           rp.Commitment.Marshal(),
					Proof:                rp.Proof.Marshal(),
					BlockRange:           rp.BlockRange,
					DependentCommitments: rp.DependentCommitments,
				}
			}

			hbBytes = make([][]byte, len(historicalBalances))
			for i, hb := range historicalBalances {
				b := hb.MarshalBinary()
				hbBytes[i] = b[:]
			}
		}
	}

	return &SamuraiQueryResult{
		Value:              s.currentKey[userKey],
		CurrentVersion:     n,
		MptProof:           mptProof,
		CommitmentHash:     commitmentHash,
		SamuraiProofs:      samuraiProofs,
		HistoricalBalances: hbBytes,
	}, nil
}

// applyUpdates flushes the updateBuffer into the segment trees, SamuraiDB,
// and MPT via storage.CreateOrUpdateAccountInfo.
func (s *SamuraiKTServer) applyUpdates() {
	s.rootCommitmentIsDirty = false

	for _, pair := range s.updateBuffer {
		userKey := string(pair.User)
		account := userToAddress(pair.User)

		s.currentKey[userKey] = common.CopyBytes(pair.Key)

		s.putCounts[userKey]++
		blockNumber := s.putCounts[userKey]

		balance := new(big.Int).SetBytes(hash.BytesToHash(pair.Key).Bytes())
		storage.CreateOrUpdateAccountInfo(account, balance, blockNumber, s.cache)
		// s.cache.C.Purge()

		ai, ok := s.cache.C.Get(account)
		if !ok {
			log.Errorf("account missing from cache after CreateOrUpdateAccountInfo for user %s", hex.EncodeToString(pair.User))
			continue
		}

		// log.Infof("saving account info for user %s", account.Hex())
		ai.Save(s.samuraiDB)

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

// proveKey generates an MPT proof for the given trie key.
func (s *SamuraiKTServer) proveKey(trieKey []byte) ([][]byte, error) {
	pc := &proofCollector{}
	if err := s.mpt.Prove(trieKey, pc); err != nil {
		return nil, err
	}
	return pc.nodes, nil
}

// makeSamuraiTrieKey builds the MPT key for a user.
func makeSamuraiTrieKey(user []byte) []byte {
	buf := make([]byte, 32)
	copy(buf, user)
	return buf
}

// Compile-time interface checks (reuse proofCollector from optiks.go).
var _ ethdb.KeyValueWriter = (*proofCollector)(nil)
