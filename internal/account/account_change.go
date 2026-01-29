package account

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

func GetModifiedAccountsByNumber(blockNumber uint64, client *rpc.Client) ([]common.Address, error) {
	var res []common.Address
	if err := client.Call(
		&res,
		"debug_getModifiedAccountsByNumber",
		hexutil.Uint64(blockNumber),
		hexutil.Uint64(blockNumber),
	); err != nil {
		return nil, err
	}
	return res, nil

}
