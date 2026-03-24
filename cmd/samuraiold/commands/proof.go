package commands

import (
	"fmt"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/utils"
)

// RunProof executes the proof generation mode.
func RunProof(addr common.Address, queryStartBlock uint64, queryEndBlock uint64, dbs []*db.SamuraiStore, precomputedData *config.PrecomputedData, cfg *config.Config) {
	shardIdx := utils.AddressToShardIndex(addr, cfg.Database.Shards)
	db := dbs[shardIdx]

	fmt.Println("Generating range proofs for account", addr.Hex())
	fmt.Println("Query start block", queryStartBlock, "Query end block", queryEndBlock)

	startingVersion, endingVersion, err := proof.BlockRangeToVersionRange(addr, queryStartBlock, queryEndBlock, db)
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

	// Note: MPT proof verification is skipped in local proof mode (no state root available).
	// Pass nil/empty MPT arguments — MPT verification is gracefully skipped when no proof nodes are provided.
	if err := proof.VerifyNewRangeProofs(addr, startingVersion, endingVersion, rangeProofs, balanceInfos, precomputedData, nil, common.Hash{}, nil); err != nil {
		log.Fatalf("Verification failed: %v", err)
	}

	start = time.Now()
	fmt.Println("Time taken to verify range proofs", time.Since(start))
}
