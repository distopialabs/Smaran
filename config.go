package main

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

type Config struct {
	// GethIPC             string
	client              *rpc.Client
	StartingBlockNumber uint64
	EndingBlockNumber   uint64
	TrackedAccounts     []common.Address
}
