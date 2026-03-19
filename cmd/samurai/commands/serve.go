package commands

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/server"
	"github.com/nepal80m/samurai/mpt/meta"
	st "github.com/nepal80m/samurai/mpt/state"
)

// RunServe starts the gRPC proof server.
func RunServe(port int, dbs []*db.SamuraiStore, precomputedData *config.PrecomputedData, cfg *config.Config, dbDir string) {
	addr := fmt.Sprintf(":%d", port)

	// Open MPT database
	mptDBDir := filepath.Join(dbDir, "mpt")
	mptStore, err := st.OpenDB(mptDBDir)
	if err != nil {
		log.Fatalf("failed to open MPT database at %s: %v", mptDBDir, err)
	}
	defer mptStore.Close()

	// Log the latest MPT state root so operators can provide it to verifiers.
	if lastBlock, err := meta.GetLast(mptStore.DiskDB); err == nil {
		if root, err := meta.GetRoot(mptStore.DiskDB, lastBlock); err == nil {
			log.Printf("MPT latest block: %d, state root: %s", lastBlock, root.Hex())
		}
	}

	proofServer := server.NewProofServer(dbs, precomputedData, cfg, mptStore)

	log.Printf("Starting Samurai gRPC server on port %d", port)
	if err := server.ListenAndServe(addr, proofServer); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
