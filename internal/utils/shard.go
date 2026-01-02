package utils

import (
	"github.com/cespare/xxhash/v2"
	"github.com/ethereum/go-ethereum/common"
)

func AddressToShardIndex(address common.Address, shards int) int {
	h := xxhash.Sum64(address[:])
	sIdx := int(h % uint64(shards))
	return sIdx
}
