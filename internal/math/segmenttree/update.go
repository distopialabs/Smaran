package segmenttree

import (
	"math/big"
	"strconv"

	"github.com/cockroachdb/pebble"
	"github.com/consensys/gnark-crypto/ecc"
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/math"
	"github.com/nepal80m/samurai/internal/math/polynomial"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

const L1BatchSize = 2048

// const L1BatchSize = 8

const L2BatchSize = 1365

// const L2BatchSize = 5

const MaxLayer = 4

const SegmentTreeSize = L1BatchSize * 2 //2048 * 2 = 4096

func (accountInfo *AccountInfo) FirstUpdate(blockNumber uint64, balance *big.Int) common.Hash {

	// current balance
	cb := &CurrentBalance{
		Version:    0,
		Balance:    balance,
		StartBlock: blockNumber,
	}
	accountInfo.CurrentBalanceInfo = cb
	// TODO: Store current balance in db

	// tree
	var latestBatchTree [MaxLayer][]common.Hash
	for i := range MaxLayer {
		latestBatchTree[i] = make([]common.Hash, SegmentTreeSize)
	}
	accountInfo.CurrentBatchTree = latestBatchTree
	// TODO: store latest batch tree in db

	// tree commitments
	var latestBatchCommitments [MaxLayer]gnark_kzg.Digest
	accountInfo.CurrentBatchTreeCommitments = latestBatchCommitments
	// TODO: store latest batch commitments in db

	// final commitment
	treeCommitHash := CommitmentToHash(latestBatchCommitments[3])
	cbHash := cb.Hash()
	commitmentHash := BytesToPoseidonHash(cbHash.Bytes(), treeCommitHash.Bytes())
	// TODO: store final commitment in db

	return commitmentHash
}

func (accountInfo *AccountInfo) Update(blockNumber uint64, balance *big.Int, db *pebble.DB) common.Hash {
	prevCb := accountInfo.CurrentBalanceInfo
	hb := prevCb.Archive(blockNumber - 1)

	// Update current balance
	cb := &CurrentBalance{
		Version:    prevCb.Version + 1,
		Balance:    balance,
		StartBlock: blockNumber,
	}
	accountInfo.CurrentBalanceInfo = cb
	StoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, db)

	// Update historical balance and segment tree
	hbBytes := hb.Bytes()
	hbHash := hb.Hash()
	_ = hbBytes
	StoreHistoricalBalance(hb, db)

	//  This will update the current batch tree and commitments inplace.
	accountInfo.AddLeafNode(hb.Version, hbHash)
	StoreCurrentBatchTree(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, &accountInfo.CurrentBatchTree, db)
	StoreCurrentBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, &accountInfo.CurrentBatchTreeCommitments, db)

	// final commitment
	treeCommitHash := CommitmentToHash(accountInfo.CurrentBatchTreeCommitments[3])
	cbHash := cb.Hash()
	commitmentHash := BytesToPoseidonHash(cbHash.Bytes(), treeCommitHash.Bytes())

	return commitmentHash
}

// updates the current batch tree, and commitments. resets them if needed.
func (accountInfo *AccountInfo) AddLeafNode(leafNodeIdx uint64, leafNodeHash common.Hash) {

	// find which index to update for each layer
	lxModIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		if layer == 1 {
			return leafNodeIdx % L1BatchSize

		} else {
			return leafNodeIdx / math.Pow(L2BatchSize, layer-1) % L2BatchSize
		}
	}

	lxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return leafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	_ = lxModIdx
	_ = lxBatchIdx

	// idx0 := blockNumber % L1BatchSize
	// idx1 := blockNumber / L1BatchSize % L2BatchSize
	// idx2 := blockNumber / (L1BatchSize * L2BatchSize) % L2BatchSize
	// idx3 := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize) % L2BatchSize

	// l1CommitIndex := blockNumber / L1BatchSize
	// l2CommitIndex := blockNumber / (L1BatchSize * L2BatchSize)
	// l3CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize)
	// l4CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize * L2BatchSize)

	// Resetting for new batch
	for layer := 1; layer <= MaxLayer; layer++ {
		if lxModIdx(uint64(layer)) == 0 {
			accountInfo.CurrentBatchTree[layer-1] = make([]common.Hash, SegmentTreeSize)
			accountInfo.CurrentBatchTreeCommitments[layer-1] = gnark_kzg.Digest{}
		}
	}
	// TODO: uncomment this and replace the below code with this.
	// lXm1CommitHash := common.Hash{}
	// lXm1RootHash := leafNodeHash
	// for layer := uint64(1); layer <= MaxLayer; layer++ {
	// 	lxCommit := accountInfo.UpdateLXTree(L1BatchSize-1+lxModIdx(layer), lXm1RootHash, lXm1CommitHash, layer)
	// 	lxCommitHash := CommitmentToHash(lxCommit)
	// 	lxRootHash := accountInfo.CurrentBatchTree[layer][0]
	// 	lXm1CommitHash = lxCommitHash
	// 	lXm1RootHash = lxRootHash
	// }

	// updating layer 1 tree of current batch and calculate its commitment
	l1Commit := accountInfo.UpdateLXTree(L1BatchSize-1+lxModIdx(1), leafNodeHash, common.Hash{}, 1)
	l1CommitHash := CommitmentToHash(l1Commit)
	l1RootHash := accountInfo.CurrentBatchTree[0][0]

	// updating layer 2

	l2Commit := accountInfo.UpdateLXTree(L2BatchSize-1+lxModIdx(2), l1RootHash, l1CommitHash, 2)
	l2CommitHash := CommitmentToHash(l2Commit)
	l2RootHash := accountInfo.CurrentBatchTree[1][0]

	// updating layer 3

	l3Commit := accountInfo.UpdateLXTree(L2BatchSize-1+lxModIdx(3), l2RootHash, l2CommitHash, 3)
	l3CommitHash := CommitmentToHash(l3Commit)
	l3RootHash := accountInfo.CurrentBatchTree[2][0]

	// updating layer 4
	l4Commit := accountInfo.UpdateLXTree(L2BatchSize-1+lxModIdx(4), l3RootHash, l3CommitHash, 4)
	l4CommitHash := CommitmentToHash(l4Commit)
	l4RootHash := accountInfo.CurrentBatchTree[3][0]
	_ = l4CommitHash
	_ = l4RootHash

}

func (accountInfo *AccountInfo) UpdateLXTree(idx uint64, val common.Hash, lXm1CommitHash common.Hash, layer uint64) bls.G1Affine {

	tree := accountInfo.CurrentBatchTree[layer-1]
	prevCommit := accountInfo.CurrentBatchTreeCommitments[layer-1]

	var newCommit bls.G1Affine
	newCommit.Set(&prevCommit)

	if layer > 1 {

		existingLXm1CommitHash := tree[L2BatchSize+idx]
		tree[L2BatchSize+idx] = lXm1CommitHash

		incCommitBigInt := lXm1CommitHash.Big()

		if (existingLXm1CommitHash != common.Hash{}) {
			incCommitBigInt.Sub(incCommitBigInt, existingLXm1CommitHash.Big())
		}

		var incCommitNew bls.G1Affine
		incCommitNew.ScalarMultiplication(&accountInfo.PrecomputedData.WeightCommits[L2BatchSize+idx], incCommitBigInt)
		newCommit.Add(&newCommit, &incCommitNew)

	}

	// updating the tree
	// note: root hash of layer 1 is empty until the whole batch is filled. instead of updating the tree with empty hash everytime, we skip the tree update unless the root is filled. this is purely for optimization.
	if (val != common.Hash{}) {
		tree[idx] = val
		updatedIndices := []uint64{idx}
		updatedXs := []uint64{idx}
		updatedYs := []*big.Int{val.Big()}
		// TODO: switched from int to uint64; check if it creates a bug here. 2025-09-04
		for idx > 0 {
			parentIdx := GetParent(idx)

			lChild := tree[2*parentIdx+1]
			rChild := tree[2*parentIdx+2]
			if (lChild == common.Hash{} || rChild == common.Hash{}) {
				break
			}
			tree[parentIdx] = BytesToPoseidonHash(lChild.Bytes(), rChild.Bytes())

			updatedIndices = append(updatedIndices, parentIdx)
			updatedXs = append(updatedXs, parentIdx)
			updatedYs = append(updatedYs, tree[parentIdx].Big())

			idx = parentIdx

		}
		if len(updatedIndices) > 7 {
			// Using multi expo for large number of updates
			points := make([]bls.G1Affine, len(updatedXs))
			scalars := make([]fr.Element, len(updatedXs))
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
	accountInfo.CurrentBatchTreeCommitments[layer-1] = newCommit

	return newCommit
}

// // TODO: move this to kzg package and take srs as argument. ps: do we need this?
// func (segmentTree *SegmentTree) _CalculateCommitment(poly polynomial.Polynomial) common.Hash {

// 	commitment, err := gnark_kzg.Commit(poly, segmentTree.PrecomputedData.SRS.Inner.Pk)
// 	if err != nil {
// 		log.Fatalf("failed to commit: %v", err)
// 	}
// 	return CommitmentToHash(commitment)
// 	// commitmentBytes := commitment.Bytes()
// 	// commitmentHash := common.BytesToHash(commitmentBytes[:])

// 	// return commitmentHash
// }

// TODO: do we need this?
func _GenerateIncrementalPolynomial(indexToProcess []int, V polynomial.Polynomial, weights []fr.Element, tree []common.Hash) polynomial.Polynomial {

	xValues := make([]int, len(indexToProcess))
	yValues := make([]fr.Element, len(indexToProcess))

	for i, index := range indexToProcess {
		xValues[i] = index
		yValues[i] = polynomial.HashToFieldElement(tree[index])
	}

	incPoly := polynomial.Interpolate(xValues, yValues, V, weights)

	return incPoly
}

// // TODO: do we need this?
// func (segmentTree *SegmentTree) _FlushIfRemaining(blockNumber int) {
// 	commitIdx := map[int]int{
// 		1: blockNumber / L1BatchSize,
// 		2: blockNumber / (L1BatchSize * L2BatchSize),
// 		3: blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize),
// 		4: blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize * L2BatchSize),
// 	}

// 	for layer := 1; layer <= MaxLayer; layer++ {
// 		for i := 0; i < SegmentTreeSize; i++ {
// 			if segmentTree.LXTreeV3[layer][i] != (common.Hash{}) {
// 				WriteTreeSegment(StoragePath, segmentTree.Account, layer, commitIdx[layer], segmentTree.LXTreeV3[layer])
// 			}
// 		}
// 	}
// }
