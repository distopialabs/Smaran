package segmenttree

import (
	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
)

func StoreAccountInfo(accountInfo *AccountInfo, db *pebble.DB) {

	StoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, db)
	StoreCurrentBatchTree(accountInfo.Account, &accountInfo.CurrentBatchTree, db)
	StoreBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, &accountInfo.CurrentBatchTreeCommitments, db)
}

// only returns error if the account info is not found, otherwise panics
func GetAccountInfo(account common.Address, db *pebble.DB) (*AccountInfo, error) {
	cbInfo, err := GetCurrentBalanceInfo(account, db)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, err
		}
		panic(err)
	}
	batchTree := GetCurrentBatchTree(account, db)
	batchCommitments := GetBatchCommitments(account, cbInfo.Version, db)
	accountInfo := &AccountInfo{
		Account:                     account,
		CurrentBalanceInfo:          cbInfo,
		CurrentBatchTree:            *batchTree,
		CurrentBatchTreeCommitments: *batchCommitments,
	}
	return accountInfo, nil
}

func SetAccountInfo(accountInfo *AccountInfo, db *pebble.DB) {
	StoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, db)
	StoreCurrentBatchTree(accountInfo.Account, &accountInfo.CurrentBatchTree, db)
	StoreBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, &accountInfo.CurrentBatchTreeCommitments, db)
}
