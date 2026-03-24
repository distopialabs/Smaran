package main

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

// fetches balances for multiple accounts in a single RPC call for a given block number
func BatchMultiUserBalance(addrs []common.Address, blockNum uint64, client *rpc.Client) ([]*big.Int, error) {
	if len(addrs) == 0 {
		return make([]*big.Int, 0), nil
	}

	elems := make([]rpc.BatchElem, len(addrs))
	for i, addr := range addrs {
		var bal hexutil.Big
		elems[i] = rpc.BatchElem{
			Method: "eth_getBalance",
			Args:   []any{addr, hexutil.Uint64(blockNum)},
			Result: &bal,
		}
	}
	if err := client.BatchCallContext(context.Background(), elems); err != nil {
		return nil, err
	}

	balances := make([]*big.Int, len(elems))
	for i, e := range elems {
		balances[i] = e.Result.(*hexutil.Big).ToInt()
	}
	return balances, nil
}

// fetches balances for a single account for a range of block numbers
func batchSingleUserBalances(addr common.Address, startBlockNum, endBlockNum uint64, client *rpc.Client) ([]*big.Int, error) {
	elems := make([]rpc.BatchElem, endBlockNum-startBlockNum+1)
	for i := range endBlockNum - startBlockNum + 1 {
		var bal hexutil.Big
		elems[i] = rpc.BatchElem{
			Method: "eth_getBalance",
			Args:   []any{addr, hexutil.Uint64(startBlockNum + uint64(i))},
			Result: &bal,
		}
	}

	if err := client.BatchCallContext(context.Background(), elems); err != nil {
		return nil, err
	}

	balances := make([]*big.Int, len(elems))
	for i, e := range elems {
		balances[i] = e.Result.(*hexutil.Big).ToInt()
	}
	return balances, nil
}
