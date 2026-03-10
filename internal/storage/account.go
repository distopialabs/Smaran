package storage

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/tree"
)

// CreateOrUpdateAccountInfo updates an account's balance and returns the commitment hash.
func CreateOrUpdateAccountInfo(account common.Address, balance *big.Int, blockNumber uint64, cache *Cache) common.Hash {
	initFn := func(account common.Address) *tree.AccountInfo {
		return tree.NewAccountInfo(account, cache.PrecomputedData)
	}

	loadFn := func(account common.Address, sdb *db.SamuraiDB) *tree.AccountInfo {
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
			PrecomputedData:          cache.PrecomputedData,
			DirtyChunks:              tree.InitDirtyChunks(),
			CurrentLXTreeCounts:      treeCounts,
			LXLeafNodes:              lxLeafNodes,
		}
	}

	mutate := func(accountInfo *tree.AccountInfo, sdb *db.SamuraiDB) {
		accountInfo.Update(blockNumber, balance, sdb)
	}

	accountInfo, err := cache.Update(account, initFn, loadFn, mutate)
	if err != nil {
		panic(err)
	}
	commitmentHash := accountInfo.CalculateFinalCommitment()
	return commitmentHash
}
