package segmenttree

import (
	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
)

func BatchStoreAccountInfo(accountInfo *AccountInfo, b *pebble.Batch) {
	BatchStoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, b)
	BatchStoreCurrentLXBatchTree(accountInfo.Account, accountInfo.CurrentLXBatchTree, b)
	BatchStoreLXBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchCommitment, b)
}

func StoreAccountInfo(accountInfo *AccountInfo, db *pebble.DB) {

	StoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, db)
	StoreCurrentLXBatchTree(accountInfo.Account, accountInfo.CurrentLXBatchTree, db)
	StoreLXBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchCommitment, db)
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
	batchTree := GetCurrentLXBatchTree(account, db)
	batchCommitments := GetLXBatchCommitments(account, cbInfo.Version, db)
	accountInfo := &AccountInfo{
		Account:                  account,
		CurrentBalanceInfo:       cbInfo,
		CurrentLXBatchTree:       batchTree,
		CurrentLXBatchCommitment: batchCommitments,
	}
	return accountInfo, nil
}

func SetAccountInfo(accountInfo *AccountInfo, db *pebble.DB) {
	StoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, db)
	StoreCurrentLXBatchTree(accountInfo.Account, accountInfo.CurrentLXBatchTree, db)
	StoreLXBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchCommitment, db)
}
