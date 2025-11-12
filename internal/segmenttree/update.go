package segmenttree

import (
	"math/big"
	"strconv"

	"github.com/consensys/gnark-crypto/ecc"
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/math"
	"github.com/nepal80m/samurai/internal/math/polynomial"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

func (accountInfo *AccountInfo) Update(blockNumber uint64, balance *big.Int, db DB) {
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

	// Update current balance
	cb := &CurrentBalance{
		Version:    prevCb.Version + 1,
		Balance:    balance,
		StartBlock: blockNumber,
	}
	accountInfo.CurrentBalanceInfo = cb

	// Update historical balance and segment tree
	// hbBytes := hb.Bytes()
	hbHash := hb.Hash()
	// StoreHistoricalBalanceByHash(hb, db)

	//  This will update the current batch tree and commitments inplace.
	// start := time.Now()
	accountInfo.AddLeafNode(hb.Version, hbHash)
	// fmt.Println("Time taken to add leaf node", time.Since(start))

	// Save
	// start = time.Now()
	StoreHistoricalBalance(accountInfo.Account, hb, db)
	// fmt.Println("Time taken to store historical balance in db", time.Since(start))

}

func (accountInfo *AccountInfo) CalculateFinalCommitment() common.Hash {
	treeCommitHash := CommitmentToHash(accountInfo.CurrentLXBatchCommitment[MaxLayer-1])
	cbHash := accountInfo.CurrentBalanceInfo.Hash()
	commitmentHash := BytesToPoseidonHash(cbHash.Bytes(), treeCommitHash.Bytes())
	return commitmentHash

}

func (accountInfo *AccountInfo) Save(db DB) {
	StoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, db)
	StoreCurrentLXBatchTree(accountInfo.Account, accountInfo.CurrentLXBatchTree, db)
	StoreLXBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchCommitment, db)
}

// updates the current batch tree, and commitments. resets them if needed.
func (accountInfo *AccountInfo) AddLeafNode(leafNodeIdx uint64, leafNodeHash common.Hash) {

	// find which index to update for each layer
	// - : reset
	// 1: 0,1,2,3,4 - 0,1,2,3,4 - 0,1,2,3,4 - 0,1,2,3,4 - 0,1,2,3,4 - 0,1,2,3,4
	// 2: 0,0,0,0,0,1,1,1,1,1,2,2,2,2,2,3,3,3,3,3,4,4,4,4,4 - 0,0,0,0,0
	// 3: 0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1,1,1,1,1
	// idx0 := blockNumber % L1BatchSize
	// idx1 := blockNumber / L1BatchSize % L2BatchSize
	// idx2 := blockNumber / (L1BatchSize * L2BatchSize) % L2BatchSize
	// idx3 := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize) % L2BatchSize

	lxBatchNodeIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		if layer == 1 {
			return leafNodeIdx % L1BatchSize

		} else {
			return leafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-2)) % L2BatchSize
		}
	}

	lxBatchLeafNodeOffsetIdx := func(layer uint64) uint64 {
		idx := lxBatchNodeIdx(layer)
		if layer == 1 {
			return L1BatchSize - 1 + idx
		} else {
			return L2BatchSize - 1 + idx
		}
	}
	_ = lxBatchLeafNodeOffsetIdx

	// 1: 0,0,0,0,0 - 1,1,1,1,1 - 2,2,2,2,2 - 3,3,3,3,3 - 4,4,4,4,4 - 0,0,0,0,0
	// 2: 0,0,0,0,0 - 0,0,0,0,0 - 0,0,0,0,0 - 0,0,0,0,0 - 0,0,0,0,0 - 1,1,1,1,1
	// 3: 0,0,0,0,0 - 0,0,0,0,0 - 0,0,0,0,0 - 0,0,0,0,0 - 0,0,0,0,0 - 0,0,0,0,0
	// l1CommitIndex := blockNumber / L1BatchSize
	// l2CommitIndex := blockNumber / (L1BatchSize * L2BatchSize)
	// l3CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize)
	// l4CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize * L2BatchSize)

	lxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return leafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	lxBatchSize := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		if layer == 1 {
			return L1BatchSize
		}
		return L2BatchSize
	}

	_ = lxBatchIdx
	_ = lxBatchSize

	// Resetting for new batch
	for layer := 1; layer <= MaxLayer; layer++ {
		if (leafNodeIdx % (L1BatchSize * math.Pow(L2BatchSize, uint64(layer)-1))) == 0 {
			accountInfo.CurrentLXBatchTree[layer-1] = BatchTree{}
			accountInfo.CurrentLXBatchCommitment[layer-1] = gnark_kzg.Digest{}
		}
	}
	// TODO: uncomment this and replace the below code with this. add if else conditionfor layer 1 and others.
	// lXm1CommitHash := common.Hash{}
	// lXm1RootHash := leafNodeHash
	// for layer := uint64(1); layer <= MaxLayer; layer++ {
	// 	lxCommit := accountInfo.UpdateLXTree(lxBatchSize(layer)-1+lxModIdx(layer), lXm1RootHash, lXm1CommitHash, layer)
	// 	lxCommitHash := CommitmentToHash(lxCommit)
	// 	lxRootHash := accountInfo.CurrentBatchTree[layer-1][0]
	// 	lXm1CommitHash = lxCommitHash
	// 	lXm1RootHash = lxRootHash
	// }

	// updating layer 1 tree of current batch and calculate its commitment
	l1Commit := accountInfo.UpdateLXTree(lxBatchLeafNodeOffsetIdx(1), leafNodeHash, common.Hash{}, 1)
	l1CommitHash := CommitmentToHash(l1Commit)
	l1RootHash := accountInfo.CurrentLXBatchTree[0][0]

	// updating layer 2
	l2Commit := accountInfo.UpdateLXTree(lxBatchLeafNodeOffsetIdx(2), l1RootHash, l1CommitHash, 2)
	l2CommitHash := CommitmentToHash(l2Commit)
	l2RootHash := accountInfo.CurrentLXBatchTree[1][0]

	// updating layer 3
	l3Commit := accountInfo.UpdateLXTree(lxBatchLeafNodeOffsetIdx(3), l2RootHash, l2CommitHash, 3)
	l3CommitHash := CommitmentToHash(l3Commit)
	l3RootHash := accountInfo.CurrentLXBatchTree[2][0]

	// updating layer 4
	l4Commit := accountInfo.UpdateLXTree(lxBatchLeafNodeOffsetIdx(4), l3RootHash, l3CommitHash, 4)
	l4CommitHash := CommitmentToHash(l4Commit)
	l4RootHash := accountInfo.CurrentLXBatchTree[3][0]
	_ = l4CommitHash
	_ = l4RootHash

}

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
	accountInfo.CurrentLXBatchCommitment[layer-1] = newCommit

	return newCommit
}
