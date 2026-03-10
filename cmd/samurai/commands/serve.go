package commands

import (
	"fmt"

	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/server"
)

// RunServe starts the gRPC proof server.
func RunServe(port int, dbs []*db.SamuraiDB, precomputedData *config.PrecomputedData, cfg *config.Config) {
	addr := fmt.Sprintf(":%d", port)

	proofServer := server.NewProofServer(dbs, precomputedData, cfg)

	log.Infof("Starting Samurai gRPC server on port %d", port)
	if err := server.ListenAndServe(addr, proofServer); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
