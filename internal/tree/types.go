// Package tree provides segment tree data structures and operations for Samurai.
package tree

import (
	"fmt"
	"math/big"
	"unsafe"

	"github.com/consensys/gnark-crypto/ecc"
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/hash"
	"github.com/nepal80m/samurai/internal/crypto/polynomial"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/utils"
)

// Tree size constants
const (
	L1BatchSize     = 2048
	L2BatchSize     = 1365
	MaxLayer        = 4
	ChunkSize       = 64
	SegmentTreeSize = L1BatchSize * 2 // 4096
)

// BatchTree is a fixed-size array of hashes representing a segment tree batch.
type BatchTree [SegmentTreeSize]common.Hash

// MarshalBinary serializes the BatchTree to bytes.
func (t *BatchTree) MarshalBinary() []byte {
	b := make([]byte, SegmentTreeSize*common.HashLength)
	for i := range SegmentTreeSize {
		copy(b[i*common.HashLength:(i+1)*common.HashLength], t[i][:])
	}
	return b
}

// UnmarshalBinary deserializes bytes into a BatchTree.
func (t *BatchTree) UnmarshalBinary(b []byte) error {
	if len(b) != SegmentTreeSize*common.HashLength {
		return fmt.Errorf("bad length: got %d, want %d", len(b), SegmentTreeSize*common.HashLength)
	}
	for i := range SegmentTreeSize {
		copy(t[i][:], b[i*common.HashLength:(i+1)*common.HashLength])
	}
	return nil
}

// AsBytesUnsafe returns the underlying byte representation without copying.
func (t *BatchTree) AsBytesUnsafe() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(&t[0])), SegmentTreeSize*common.HashLength)
}

// LXBatchTree is an array of BatchTrees for each layer.
type LXBatchTree [MaxLayer]BatchTree

// LXBatchCommitment is an array of KZG commitments for each layer.
type LXBatchCommitment [MaxLayer]gnark_kzg.Digest

// DeepCopy creates a deep copy of the LXBatchTree.
func (t *LXBatchTree) DeepCopy() *LXBatchTree {
	c := *t
	return &c
}

// DeepCopy creates a deep copy of the LXBatchCommitment.
func (t *LXBatchCommitment) DeepCopy() *LXBatchCommitment {
	c := *t
	return &c
}

// InitDirtyChunks creates an initialized array of dirty chunk maps.
func InitDirtyChunks() [MaxLayer]map[int]bool {
	var dirtyChunks [MaxLayer]map[int]bool
	for i := 0; i < MaxLayer; i++ {
		dirtyChunks[i] = make(map[int]bool)
	}
	return dirtyChunks
}

// AccountInfo holds all state for a single account.
type AccountInfo struct {
	Account                  common.Address
	CurrentBalanceInfo       *CurrentBalance
	CurrentLXBatchTree       *LXBatchTree
	CurrentLXBatchCommitment *LXBatchCommitment
	DirtyChunks              [MaxLayer]map[int]bool
	PrecomputedData          *config.PrecomputedData
}

// NewAccountInfo creates a new AccountInfo with initialized tree structures.
func NewAccountInfo(account common.Address, precomputedData *config.PrecomputedData) *AccountInfo {
	return &AccountInfo{
		Account:                  account,
		CurrentLXBatchTree:       new(LXBatchTree),
		CurrentLXBatchCommitment: new(LXBatchCommitment),
		DirtyChunks:              InitDirtyChunks(),
		PrecomputedData:          precomputedData,
	}
}

// DeepCopy creates a deep copy of the AccountInfo.
func (a *AccountInfo) DeepCopy() *AccountInfo {
	return &AccountInfo{
		Account:                  a.Account,
		CurrentBalanceInfo:       a.CurrentBalanceInfo.DeepCopy(),
		CurrentLXBatchTree:       a.CurrentLXBatchTree.DeepCopy(),
		CurrentLXBatchCommitment: a.CurrentLXBatchCommitment.DeepCopy(),
		PrecomputedData:          a.PrecomputedData,
	}
}

// Update updates the account with a new balance at the given block.
func (accountInfo *AccountInfo) Update(blockNumber uint64, balance *big.Int, sdb *db.SamuraiDB) {
	prevCb := accountInfo.CurrentBalanceInfo

	if prevCb == nil {
		accountInfo.CurrentBalanceInfo = &CurrentBalance{
			Version:    0,
			Balance:    balance,
			StartBlock: blockNumber,
		}
		return
	}
	hb := prevCb.ToHistoricalBalance(blockNumber - 1)

	// Store historical balance for proof generation
	StoreHistoricalBalance(accountInfo.Account, hb, sdb.HistoryDB)

	// Update current balance
	cb := &CurrentBalance{
		Version:    prevCb.Version + 1,
		Balance:    balance,
		StartBlock: blockNumber,
	}
	accountInfo.CurrentBalanceInfo = cb

	// Update historical balance and segment tree
	hbHash := hb.Hash()
	accountInfo.AddLeafNode(hb.Version, hbHash)

	if cb.Version > 0 && cb.Version%L1BatchSize == 0 {
		// explicitly persist the finalized commitment and root before the *next* batch boundary resets them in memory
		StoreLXBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchCommitment, sdb.StateDB)
		StoreLXBatchRoots(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchTree, sdb.StateDB)
	}
}

// CalculateFinalCommitment computes the final commitment hash.
func (accountInfo *AccountInfo) CalculateFinalCommitment() common.Hash {
	// treeCommitHash := hash.CommitmentToHash(accountInfo.CurrentLXBatchCommitment[MaxLayer-1])
	treeCommitHash := hash.CommitmentToHash(accountInfo.CurrentLXBatchCommitment[MaxLayer-1])
	cbHash := accountInfo.CurrentBalanceInfo.Hash()
	// commitmentHash := hash.BytesToPoseidonHash(cbHash.Bytes(), treeCommitHash.Bytes())
	commitmentHash := hash.BytesToHash(cbHash.Bytes(), treeCommitHash.Bytes())
	return commitmentHash
}

// Save persists the account to the database.
func (accountInfo *AccountInfo) Save(sdb *db.SamuraiDB) {
	StoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, sdb.StateDB)
	StoreCurrentLXBatchTree(accountInfo.Account, accountInfo.CurrentLXBatchTree, &accountInfo.DirtyChunks, sdb.TreeDB)
	StoreLXBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchCommitment, sdb.StateDB)
	StoreLXBatchRoots(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchTree, sdb.StateDB)
}

// AddLeafNode updates the tree with a new leaf node.
func (accountInfo *AccountInfo) AddLeafNode(leafNodeIdx uint64, leafNodeHash common.Hash) {
	lxBatchLeafNodeOffsetIdx := func(layer uint64) uint64 {
		idx := func() uint64 {
			if layer == 0 || layer > MaxLayer {
				panic("layer not supported")
			}
			if layer == 1 {
				return leafNodeIdx % L1BatchSize
			}
			return leafNodeIdx / (L1BatchSize * utils.PowUint64(L2BatchSize, layer-2)) % L2BatchSize
		}()
		if layer == 1 {
			return L1BatchSize - 1 + idx
		}
		return L2BatchSize - 1 + idx
	}

	// Resetting for new batch
	for layer := 1; layer <= MaxLayer; layer++ {
		if (leafNodeIdx % (L1BatchSize * utils.PowUint64(L2BatchSize, uint64(layer)-1))) == 0 {
			accountInfo.CurrentLXBatchTree[layer-1] = BatchTree{}
			accountInfo.CurrentLXBatchCommitment[layer-1] = gnark_kzg.Digest{}
		}
	}

	lXm1CommitHash := common.Hash{}
	lXm1RootHash := leafNodeHash
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		lxCommit := accountInfo.UpdateLXTree(lxBatchLeafNodeOffsetIdx(layer), lXm1RootHash, lXm1CommitHash, layer)
		// lxCommitHash := hash.CommitmentToHash(lxCommit)
		lxCommitHash := hash.CommitmentToHash(lxCommit)
		lxRootHash := accountInfo.CurrentLXBatchTree[layer-1][0]
		lXm1CommitHash = lxCommitHash
		lXm1RootHash = lxRootHash
	}
}

// UpdateLXTree updates a layer of the tree.
func (accountInfo *AccountInfo) UpdateLXTree(idx uint64, val common.Hash, lXm1CommitHash common.Hash, layer uint64) bls.G1Affine {
	tree := &accountInfo.CurrentLXBatchTree[layer-1]
	prevCommit := accountInfo.CurrentLXBatchCommitment[layer-1]

	var newCommit bls.G1Affine
	newCommit.Set(&prevCommit)

	if accountInfo.PrecomputedData == nil {
		panic("precomputed data is nil")
	}

	if layer > 1 {
		existingLXm1CommitHash := tree[L2BatchSize+idx]
		tree[L2BatchSize+idx] = lXm1CommitHash
		chunkIdx := int((L2BatchSize + idx) / ChunkSize)
		accountInfo.DirtyChunks[layer-1][chunkIdx] = true

		incCommitBigInt := lXm1CommitHash.Big()
		if (existingLXm1CommitHash != common.Hash{}) {
			incCommitBigInt.Sub(incCommitBigInt, existingLXm1CommitHash.Big())
		}

		var incCommitNew bls.G1Affine
		incCommitNew.ScalarMultiplication(&accountInfo.PrecomputedData.WeightCommits[L2BatchSize+idx], incCommitBigInt)
		newCommit.Add(&newCommit, &incCommitNew)
	}

	if (val != common.Hash{}) {
		tree[idx] = val
		chunkIdx := int(idx / ChunkSize)
		accountInfo.DirtyChunks[layer-1][chunkIdx] = true

		updatedIndices := []uint64{idx}
		updatedYs := []*big.Int{val.Big()}

		for idx > 0 {
			parentIdx := GetParent(idx)
			lChild := tree[2*parentIdx+1]
			rChild := tree[2*parentIdx+2]
			if (lChild == common.Hash{} || rChild == common.Hash{}) {
				break
			}
			// tree[parentIdx] = hash.BytesToPoseidonHash(lChild.Bytes(), rChild.Bytes())
			tree[parentIdx] = hash.BytesToHash(lChild.Bytes(), rChild.Bytes())

			chunkIdx := int(parentIdx / ChunkSize)
			accountInfo.DirtyChunks[layer-1][chunkIdx] = true

			updatedIndices = append(updatedIndices, parentIdx)
			updatedYs = append(updatedYs, tree[parentIdx].Big())
			idx = parentIdx
		}

		if len(updatedIndices) > 7 {
			points := make([]bls.G1Affine, len(updatedIndices))
			scalars := make([]fr.Element, len(updatedIndices))
			for i, idx := range updatedIndices {
				points[i] = accountInfo.PrecomputedData.WeightCommits[idx]
				scalars[i] = polynomial.HashToFieldElement(tree[idx])
			}
			var tempIncCommit bls.G1Affine
			tempIncCommit.MultiExp(points, scalars, ecc.MultiExpConfig{})
			newCommit.Add(&newCommit, &tempIncCommit)
		} else {
			for i, idx := range updatedIndices {
				var incCommit bls.G1Affine
				incCommit.ScalarMultiplication(&accountInfo.PrecomputedData.WeightCommits[idx], updatedYs[i])
				newCommit.Add(&newCommit, &incCommit)
			}
		}
	}

	accountInfo.CurrentLXBatchCommitment[layer-1] = newCommit
	return newCommit
}
