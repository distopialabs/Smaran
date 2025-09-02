package config

import (
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

type Config struct {
	// GethIPC             string
	Client              *rpc.Client
	StartingBlockNumber uint64
	EndingBlockNumber   uint64
	TrackedAccounts     []common.Address
}

func (config *Config) SetTrackedAccounts(count int) []common.Address {
	client := config.Client

	accountAddrs := make([]common.Address, 0, count)
	startKey := []byte{}
	for {
		var iteratorDump struct {
			Root     string                 `json:"root"`
			Accounts map[common.Address]any `json:"accounts"`
			Next     []byte                 `json:"next"`
		}
		blockNumber := config.StartingBlockNumber
		const batchSize = 256
		if err := client.Call(
			&iteratorDump,
			"debug_accountRange",
			blockNumber, // numeric block tag
			startKey,    // starting key for pagination
			batchSize,   // how many accounts to fetch per page
			true,        // exclude code info in account?
			true,        // exclude storage info in account?
		); err != nil {
			log.Fatalf("RPC error calling debug_accountRange: %v", err)
		}

		for addr := range iteratorDump.Accounts {
			if len(accountAddrs) >= count {
				break
			}
			accountAddrs = append(accountAddrs, addr)
		}
		if len(accountAddrs) >= count || len(iteratorDump.Next) == 0 {
			break
		}
		startKey = iteratorDump.Next
	}
	config.TrackedAccounts = accountAddrs
	return accountAddrs

}
