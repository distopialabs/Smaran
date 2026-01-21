package segmenttree

import (
	"github.com/ethereum/go-ethereum/common"
)

func BatchStoreAccountInfo(accountInfo *AccountInfo, b Batch) {
	BatchStoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, b)
	BatchStoreCurrentLXBatchTree(accountInfo.Account, accountInfo.CurrentLXBatchTree, &accountInfo.DirtyChunks, b)
	BatchStoreLXBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchCommitment, b)
}

func StoreAccountInfo(accountInfo *AccountInfo, db DB) {

	StoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, db)
	StoreCurrentLXBatchTree(accountInfo.Account, accountInfo.CurrentLXBatchTree, &accountInfo.DirtyChunks, db)
	StoreLXBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchCommitment, db)
}

// only returns error if the account info is not found, otherwise panics
func GetAccountInfo(account common.Address, db DB) (*AccountInfo, error) {
	cbInfo, err := GetCurrentBalanceInfo(account, db)
	if err != nil {
		if err == ErrNotFound {
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

func SetAccountInfo(accountInfo *AccountInfo, db DB) {
	StoreCurrentBalanceInfo(accountInfo.Account, accountInfo.CurrentBalanceInfo, db)
	StoreCurrentLXBatchTree(accountInfo.Account, accountInfo.CurrentLXBatchTree, nil, db)
	StoreLXBatchCommitments(accountInfo.Account, accountInfo.CurrentBalanceInfo.Version, accountInfo.CurrentLXBatchCommitment, db)
}
