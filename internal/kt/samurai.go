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
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/fft"
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
	"github.com/nepal80m/samurai/internal/utils"

	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
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

	mpt             *trie.Trie
	trieDB          *triedb.Database
	samuraiAccounts map[string]*tree.AccountInfo

	currentKey     map[string][]byte
	currentVersion map[string]uint64

	precomputedData *config.PrecomputedData
	batchSize       uint64
	keysUpdated     atomic.Uint64

	epoch atomic.Uint64
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

	s := &SamuraiKTServer{
		updateBuffer:          make([]OptiksKVPair, 0),
		rootCommitment:        types.EmptyRootHash,
		rootCommitmentIsDirty: false,
		mpt:                   mpt,
		trieDB:                mptDB,
		samuraiAccounts:       make(map[string]*tree.AccountInfo),
		currentKey:            make(map[string][]byte),
		precomputedData:       precomputedData,
		batchSize:             batchSize,
		epoch:                 atomic.Uint64{},
		currentVersion:        make(map[string]uint64),
	}
	s.keysUpdated.Store(0)
	s.epoch.Store(0)
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

func (s *SamuraiKTServer) GetTreeMaps(ai *tree.AccountInfo, lxRequiredBatchIdxs map[uint64][]uint64, startingVersion, endingVersion uint64) (map[string]*tree.BatchTree, []*tree.HistoricalBalance) {
	requiredTreeBatchesMap := make(map[string]*tree.BatchTree)
	requiredHBInfos := make([]*tree.HistoricalBalance, 0)

	numHistorical := uint64(len(ai.HistoricalBalances))

	for i, hbInfo := range ai.HistoricalBalances {
		if hbInfo.Version >= startingVersion && hbInfo.Version <= endingVersion {
			requiredHBInfos = append(requiredHBInfos, hbInfo)
		}

		version := hbInfo.Version
		nextVersion := version + 1
		for layer := uint64(1); layer <= tree.MaxLayer; layer++ {
			batchSize := tree.L1BatchSize * utils.PowUint64(tree.L2BatchSize, layer-1)
			currentBatchIdx := version / batchSize
			nextBatchIdx := nextVersion / batchSize
			isLastInBatch := uint64(i) == numHistorical-1 || (nextBatchIdx != currentBatchIdx)
			if isLastInBatch && slices.Contains(lxRequiredBatchIdxs[layer], currentBatchIdx) {
				key := fmt.Sprintf("%d:%d", layer, currentBatchIdx)
				requiredTreeBatchesMap[key] = &ai.CurrentLXBatchTree[layer-1]
			}
		}
	}

	return requiredTreeBatchesMap, requiredHBInfos
}

func (s *SamuraiKTServer) GetProofRangeInMemory(account common.Address, startingVersion, endingVersion uint64, userKey string) ([]*proof.RangeProof, []*tree.HistoricalBalance) {
	ai, ok := s.samuraiAccounts[account.Hex()]
	if !ok {
		return nil, nil
	}

	reqCommits := proof.FindCommitmentsCoveringRange(int(startingVersion), int(endingVersion))

	lxRequiredBatchIdxs := make(map[uint64][]uint64)
	for i := uint64(1); i <= tree.MaxLayer; i++ {
		lxRequiredBatchIdxs[i] = make([]uint64, 0)
	}
	for _, reqCommit := range reqCommits {
		lxRequiredBatchIdxs[uint64(reqCommit.Layer)] = append(lxRequiredBatchIdxs[uint64(reqCommit.Layer)], uint64(reqCommit.Idx))
	}
	requiredTreeBatchesMap, requiredHBInfos := s.GetTreeMaps(ai, lxRequiredBatchIdxs, startingVersion, endingVersion)

	allRangeProofs := make([]*proof.RangeProof, len(reqCommits))
	// var wg sync.WaitGroup

	for i, reqCommit := range reqCommits {
		// wg.Add(1)
		// go func(i int, reqCommit proof.RangeCommitment) {
		// 	defer wg.Done()

		layer := reqCommit.Layer
		idx := reqCommit.Idx

		nodesToInterpolate := proof.FindNodesToInterpolate(reqCommit, true)
		// log.Debugf("layer: %d, idx: %d", reqCommit.Layer, reqCommit.Idx)
		if reqCommit.BlockRange == nil {
			// log.Debugf("Commitment is not covering any range.")
		} else {
			// log.Debugf("sb: %d, eb: %d", reqCommit.BlockRange.Start, reqCommit.BlockRange.End)
		}
		// log.Debugf("DependentCommitments: %v", reqCommit.DependentCommitments)
		// log.Debugf("nodesToInterpolate: %v", nodesToInterpolate)

		// start := time.Now()
		treeKey := fmt.Sprintf("%d:%d", layer, idx)
		batchTree, ok := requiredTreeBatchesMap[treeKey]
		if !ok {
			panic(fmt.Errorf("batch tree not found for key: %s %v", treeKey, requiredTreeBatchesMap))
		}

		P := make([]fr.Element, len(batchTree))
		for i, v := range batchTree {
			P[i] = polynomial.HashToFieldElement(v)
		}
		domain := fft.NewDomain(uint64(len(P)))
		// fft.BitReverse(P)
		domain.FFTInverse(P, fft.DIF)
		fft.BitReverse(P)
		// log.Infof("Time to P: %dms", time.Since(start).Milliseconds())

		// start = time.Now()
		storedCommitment := ai.CurrentLXBatchCommitment[layer-1] // We are always going to query the entire range, so the commitment to use is always the current commitment of the layer.
		Z := polynomial.VanishingPolynomial(nodesToInterpolate, &domain.Generator)
		// log.Infof("Time to Z: %dms", time.Since(start).Milliseconds())

		// start = time.Now()
		I := make([]fr.Element, int(domain.Cardinality))
		for _, v := range nodesToInterpolate {
			I[v] = polynomial.HashToFieldElement(batchTree[v])
		}
		// fft.BitReverse(I)
		domain.FFTInverse(I, fft.DIF)
		fft.BitReverse(I)
		// log.Infof("Time to I: %dms", time.Since(start).Milliseconds())

		// start = time.Now()
		diff := kzg.SubtractPolys(P, I)
		Q := kzg.PolyDiv(diff, Z)
		QCommit, err := gnark_kzg.Commit(Q, s.precomputedData.SRS.Inner.Pk)
		if err != nil {
			panic(err)
		}
		// log.Infof("Time to generate witness: %dms", time.Since(start).Milliseconds())

		rangeProof := &proof.RangeProof{
			Idx:                  idx,
			Layer:                layer,
			Commitment:           storedCommitment,
			Proof:                QCommit,
			BlockRange:           reqCommit.BlockRange,
			DependentCommitments: reqCommit.DependentCommitments,
		}

		allRangeProofs[i] = rangeProof
		// }(i, reqCommit)
	}

	// wg.Wait()
	return allRangeProofs, requiredHBInfos
}

// getResult builds the SamuraiQueryResult for a user.
func (s *SamuraiKTServer) getResult(user []byte) (*SamuraiQueryResult, error) {
	userKey := string(user)
	n := s.currentVersion[userKey]

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
		ai, ok := s.samuraiAccounts[account.Hex()]
		if !ok {
			return nil, fmt.Errorf("account not found in cache for user %s", hex.EncodeToString(user))
		}

		commitmentHash = hash.CommitmentToHash(ai.CurrentLXBatchCommitment[tree.MaxLayer-1]).Bytes()

		version := s.currentVersion[userKey]
		if version > 0 {
			// start := time.Now()
			rangeProofs, historicalBalances := s.GetProofRangeInMemory(account, 0, version-1, userKey)
			// log.Infof("Time taken to get proof range: %dms", time.Since(start).Milliseconds())

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
			// Test, see if the verification works
			// proof.VerifyNewRangeProofs(account, 0, version-1, rangeProofs, historicalBalances, s.precomputedData)
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

func (s *SamuraiKTServer) GetAccountOrCreate(account common.Address) *tree.AccountInfo {
	if _, ok := s.samuraiAccounts[account.Hex()]; !ok {
		s.samuraiAccounts[account.Hex()] = tree.NewAccountInfo(account, s.precomputedData)
	}
	return s.samuraiAccounts[account.Hex()]
}

func (s *SamuraiKTServer) CreateOrUpdateAccountInfo(account common.Address, balance *big.Int, epoch uint64) *tree.AccountInfo {
	if _, ok := s.samuraiAccounts[account.Hex()]; !ok {
		s.samuraiAccounts[account.Hex()] = tree.NewAccountInfo(account, s.precomputedData)
	}

	s.samuraiAccounts[account.Hex()].UpdateInMemory(epoch, balance)
	return s.samuraiAccounts[account.Hex()]
}

type workerUpdate struct {
	balance *big.Int
	epoch   uint64
	account string
}

func updateWorker(accounts *map[string]*tree.AccountInfo, updateChan chan workerUpdate, wg *sync.WaitGroup) {
	defer wg.Done()
	for update := range updateChan {
		ai, ok := (*accounts)[update.account]
		if !ok {
			continue
		}
		ai.UpdateInMemory(update.epoch, update.balance)
	}
}

// applyUpdates flushes the updateBuffer into the segment trees, SamuraiDB,
// and MPT via storage.CreateOrUpdateAccountInfo.
func (s *SamuraiKTServer) applyUpdates() {
	s.rootCommitmentIsDirty = false
	epoch := s.epoch.Add(1)

	updatedAccounts := make(map[string]*tree.AccountInfo)

	for _, pair := range s.updateBuffer {
		userKey := string(pair.User)
		account := userToAddress(pair.User)

		s.currentKey[userKey] = common.CopyBytes(pair.Key)

		if _, ok := s.currentVersion[userKey]; !ok {
			s.currentVersion[userKey] = 0
		}
		s.currentVersion[userKey]++

		ai := s.GetAccountOrCreate(account)
		updatedAccounts[string(pair.User)] = ai
	}

	num_workers := runtime.NumCPU()
	// Divide the updated accounts into num_workers chunks
	chunks := make([]map[string]*tree.AccountInfo, num_workers)
	user_to_worker_map := make(map[string]int)
	for i := 0; i < num_workers; i++ {
		chunks[i] = make(map[string]*tree.AccountInfo)
	}
	i := 0
	for userKey, ai := range updatedAccounts {
		if _, ok := user_to_worker_map[userKey]; !ok {
			chunks[i%num_workers][userKey] = ai
			user_to_worker_map[userKey] = i % num_workers
			i++
		} // Else, the user is already in a chunk.
	}

	// Spawn num_workers workers
	updateChans := make([]chan workerUpdate, num_workers)
	var wg sync.WaitGroup
	for i := 0; i < num_workers; i++ {
		updateChans[i] = make(chan workerUpdate, s.batchSize)
		wg.Add(1)
		go updateWorker(&chunks[i], updateChans[i], &wg)
	}

	// Send updates to the workers

	for _, pair := range s.updateBuffer {
		userKey := string(pair.User)
		balance := new(big.Int).SetBytes(hash.BytesToHash(pair.Key).Bytes())
		updateChans[user_to_worker_map[userKey]] <- workerUpdate{
			balance: balance,
			epoch:   epoch,
			account: userKey,
		}
	}

	for i := 0; i < num_workers; i++ {
		close(updateChans[i])
	}
	wg.Wait()

	for userKey, ai := range updatedAccounts {
		user := []byte(userKey)
		commitmentHash := hash.CommitmentToHash(ai.CurrentLXBatchCommitment[tree.MaxLayer-1])
		trieKey := makeSamuraiTrieKey(user)
		trieValue, err := rlp.EncodeToBytes(commitmentHash.Bytes())
		if err != nil {
			log.Errorf("RLP encode failed for user %s: %v", hex.EncodeToString(user), err)
			continue
		}
		if err := s.mpt.Update(trieKey, trieValue); err != nil {
			log.Errorf("MPT update failed for user %s: %v", hex.EncodeToString(user), err)
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
