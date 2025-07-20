package main

import (
	"math/big"

	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
)

func blockExists(n int) bool {
	return n < 22000000
}

type Commitments struct {
	L1Commitment gnark_kzg.Digest
	L2Commitment gnark_kzg.Digest
	L3Commitment gnark_kzg.Digest
	L4Commitment gnark_kzg.Digest
}
type AccountCommitments = map[common.Hash]Commitments
type CommitmentsByBlock = map[int]AccountCommitments
type AccountBalances = map[common.Hash]*big.Int
type BalancesByBlock = map[int]AccountBalances

type SegmentTree struct {
	layer1Tree []common.Hash
	layer2Tree []common.Hash
	layer3Tree []common.Hash
	layer4Tree []common.Hash
}

func layer1Operations() {}
func MainLoop() {
	// _accountBalance := big.NewInt(int64(18000000000000000))
	// _accountBalances := make(AccountBalances)
	// _accountBalances[common.HexToHash("0x16bD8c7297df5Aa981B328DBa02466bc7c064EB7")] = _accountBalance

	// balancesByBlock := make(BalancesByBlock)
	// balancesByBlock[1] = _accountBalances

	// l1Balances := make([]*big.Int, 2048)
	// l2Balances := make([]*big.Int, 2048)
	// l3Balances := make([]*big.Int, 2048)
	// l4Balances := make([]*big.Int, 2048)

	// l1SegmentTree := make([]common.Hash, 4096)
	// l2SegmentTree := make([]common.Hash, 4096)
	// l3SegmentTree := make([]common.Hash, 4096)
	// l4SegmentTree := make([]common.Hash, 4096)

	for blockNumber := 0; blockExists(blockNumber); blockNumber++ {
		// idx0 := blockNumber % 2048
		// idx1 := blockNumber / 2048 % 1365
		// idx2 := blockNumber / (2048 * 1365) % 1365
		// idx3 := blockNumber / (2048 * 1365 * 1365) % 1365

		segmentTree := new(SegmentTree)
		if blockNumber%2048 == 0 {
			segmentTree.layer1Tree = make([]common.Hash, 4096)
		}
		if blockNumber%1024 == 0 {
			segmentTree.layer2Tree = make([]common.Hash, 4096)
		}
		if blockNumber%512 == 0 {
			segmentTree.layer3Tree = make([]common.Hash, 4096)
		}
	}
}
