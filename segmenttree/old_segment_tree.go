package segmenttree

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type SegmentTree = []common.Hash

func New(balances []*big.Int) (SegmentTree, error) {
	if len(balances) == 0 {
		return nil, fmt.Errorf("array cannot be empty")
	}
	segmentTree := make(SegmentTree, 4096)

	for i, v := range balances {
		if v == nil {
			segmentTree[len(balances)-1+i] = common.Hash{}
		} else {
			segmentTree[len(balances)-1+i] = common.BigToHash(v)
		}
	}

	for i := len(balances) - 2; i >= 0; i-- {
		lChild := segmentTree[2*i+1]
		rChild := segmentTree[2*i+2]
		if (lChild != common.Hash{} && rChild != common.Hash{}) {
			segmentTree[i] = crypto.Keccak256Hash(
				segmentTree[2*i+1].Bytes(),
				segmentTree[2*i+2].Bytes(),
			)
		}

		// segmentTree[i] = segmentTree[2*i+1] + segmentTree[2*i+2]
	}

	return segmentTree, nil

}
