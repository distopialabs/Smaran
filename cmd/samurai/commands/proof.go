package commands

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/utils"
)

// RunProof executes the proof generation mode.
func RunProof(addr common.Address, queryStartBlock uint64, queryEndBlock uint64, dbs []*db.SamuraiDB, precomputedData *config.PrecomputedData, cfg *config.Config) {
	shardIdx := utils.AddressToShardIndex(addr, cfg.Database.Shards)
	db := dbs[shardIdx]

	fmt.Println("Generating range proofs for account", addr.Hex())
	fmt.Println("Query start block", queryStartBlock, "Query end block", queryEndBlock)

	startingVersion, endingVersion, err := proof.BlockRangeToVersionRange(addr, queryStartBlock, queryEndBlock, cfg, db)
	if err != nil {
		fmt.Printf("Error: Failed to convert block range [%d, %d] to version range for account %s\n", queryStartBlock, queryEndBlock, addr.Hex())
		fmt.Printf("Reason: %v\n", err)
		fmt.Println("Hint: Ensure the query block range overlaps with the account's recorded history.")
		return
	}
	fmt.Printf("Resolved version range: startVersion=%d, endVersion=%d\n", startingVersion, endingVersion)

	start := time.Now()
	rangeProofs, balanceInfos := proof.GetNewProofRange(addr, startingVersion, endingVersion, precomputedData, db)
	fmt.Println("Time taken to generate range proofs", time.Since(start))

	fmt.Println("Range proofs", rangeProofs)
	// for _, b := range balanceInfos {
	// 	fmt.Println("Balance info", b.Version, b.StartBlock, b.EndBlock, b.Balance)
	// 	// fmt.Println("Balance info", b.Version, hexutil.EncodeUint64(b.StartBlock), hexutil.EncodeUint64(b.EndBlock), hexutil.EncodeBig(b.Balance))
	// }

	// balances := make([]*big.Int, int(queryEndBlock-queryStartBlock+1))
	// for _, b := range balanceInfos {
	// 	start := b.StartBlock
	// 	if start < queryStartBlock {
	// 		start = queryStartBlock
	// 	}
	// 	end := b.EndBlock
	// 	if end > queryEndBlock {
	// 		end = queryEndBlock
	// 	}
	// 	for j := start; j <= end; j++ {
	// 		balances[int(j-queryStartBlock)] = b.Balance
	// 	}
	// }
	// proof.VerifyRangeProofs(int(queryStartBlock), int(queryEndBlock), rangeProofs, balances, precomputedData.V, precomputedData.Weights, precomputedData.SRS)

	proof.VerifyNewRangeProofs(addr, startingVersion, endingVersion, rangeProofs, balanceInfos, precomputedData)

	start = time.Now()
	fmt.Println("Time taken to verify range proofs", time.Since(start))
}
