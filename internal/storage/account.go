package storage

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/tree"
)

// CreateOrUpdateAccountInfo updates an account's balance and returns the commitment hash.
func CreateOrUpdateAccountInfo(account common.Address, balance *big.Int, blockNumber uint64, cache *Cache) (common.Hash, error) {
	initFn := func(account common.Address) *tree.AccountInfo {
		return tree.NewAccountInfo(account, cache.PrecomputedData)
	}

	loadFn := func(account common.Address, sdb *db.SamuraiStore) *tree.AccountInfo {
		cbInfo, err := tree.GetCurrentBalanceInfo(account, &sdb.StateDB)
		if err != nil {
			if err != db.ErrNotFound {
				panic(err)
			}
			return nil
		}
		batchTree := tree.GetCurrentLXBatchTree(account, &sdb.TreeDB)
		batchCommitments := tree.GetLXBatchCommitments(account, cbInfo.Version, &sdb.StateDB)
		return &tree.AccountInfo{
			Account:                  account,
			CurrentBalanceInfo:       cbInfo,
			CurrentLXBatchTree:       batchTree,
			CurrentLXBatchCommitment: batchCommitments,
			PrecomputedData:          cache.PrecomputedData,
			DirtyChunks:              tree.InitDirtyChunks(),
		}
	}

	mutate := func(accountInfo *tree.AccountInfo, sdb *db.SamuraiStore) {
		accountInfo.Update(blockNumber, balance, sdb)
	}

	accountInfo, err := cache.Update(account, initFn, loadFn, mutate)
	if err != nil {
		panic(err)
	}
	commitmentHash := accountInfo.CalculateFinalCommitment()
	return commitmentHash, nil
}
