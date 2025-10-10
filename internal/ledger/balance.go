package ledger

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/nepal80m/samurai/internal/config"
)

func BatchMultiUserBalance(addrs []common.Address, blockNum uint64, config *config.Config) ([]*big.Int, error) {
	if len(addrs) == 0 {
		return make([]*big.Int, 0), nil
	}
	client := config.Client

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

func batchSingleUserBalances(addr common.Address, startBlockNum, endBlockNum uint64, config *config.Config) ([]*big.Int, error) {
	client := config.Client

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
