package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nepal80m/samurai/internal/verkle/proof"
	"github.com/nepal80m/samurai/internal/verkle/store"
	verkle "github.com/ethereum/go-verkle"
	"github.com/spf13/cobra"
)

func getproofCmd() *cobra.Command {
	var (
		dbDir     string
		block     uint64
		address   string
		verify    bool
		cold      bool
		dbBackend string
	)

	cmd := &cobra.Command{
		Use:   "getproof",
		Short: "Generate a Verkle balance proof for an address at a block",
		RunE: func(cmd *cobra.Command, args []string) error {
			addrStr := strings.TrimPrefix(address, "0x")
			addrBytes, err := hex.DecodeString(addrStr)
			if err != nil || len(addrBytes) != 20 {
				return fmt.Errorf("invalid address: %s (must be 20-byte hex)", address)
			}
			var addr [20]byte
			copy(addr[:], addrBytes)

			return runGetProof(dbDir, dbBackend, block, addr, verify, cold)
		},
	}

	cmd.Flags().StringVar(&dbDir, "db-dir", "", "Path to persistent DB directory (required)")
	cmd.Flags().Uint64Var(&block, "block", 0, "Block number")
	cmd.Flags().StringVar(&address, "address", "", "Address (0x-prefixed hex)")
	cmd.Flags().BoolVar(&verify, "verify", false, "Also verify the proof")
	cmd.Flags().BoolVar(&cold, "cold", false, "Cold mode: reopen DB for each operation")
	cmd.Flags().StringVar(&dbBackend, "db-backend", "pebble", "DB backend: pebble or leveldb")
	cmd.MarkFlagRequired("db-dir")
	cmd.MarkFlagRequired("block")
	cmd.MarkFlagRequired("address")

	return cmd
}

func runGetProof(dbDir, dbBackend string, block uint64, addr [20]byte, doVerify, cold bool) error {
	// Open DB
	kv, err := store.OpenKVStore(dbBackend, dbDir)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	if cold {
		// Cold mode: close and reopen to clear caches
		kv.Close()
		kv, err = store.OpenKVStore(dbBackend, dbDir)
		if err != nil {
			return fmt.Errorf("reopen db (cold): %w", err)
		}
	}
	defer kv.Close()

	ns := store.NewNodeStore(kv)
	resolver := ns.NodeResolverFn()

	// Load tree from persisted DB nodes (fast)
	root, err := ns.LoadTree(block)
	if err != nil {
		return fmt.Errorf("load tree for block %d: %w", block, err)
	}
	rootBytes := proof.SerializeCommitment(root)

	bench := &proof.BenchResult{}

	// Generate proof
	result, metrics, err := proof.GenerateProof(root, addr, rootBytes, resolver)
	if err != nil {
		return fmt.Errorf("generate proof: %w", err)
	}

	bench.ProofGenTime = time.Duration(metrics.ProofGenTimeNs)
	bench.ProofPayloadSize = metrics.ProofPayloadBytesLen
	bench.ProofJSONSize = metrics.ProofJSONBytesLen

	// Output JSON to stdout
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("output JSON: %w", err)
	}

	// Metrics to stderr
	fmt.Fprintf(os.Stderr, "proof_gen_time_ns=%d\n", metrics.ProofGenTimeNs)
	fmt.Fprintf(os.Stderr, "json_marshal_time_ns=%d\n", metrics.JSONMarshalTimeNs)
	fmt.Fprintf(os.Stderr, "proof_json_bytes_len=%d\n", metrics.ProofJSONBytesLen)
	fmt.Fprintf(os.Stderr, "proof_payload_bytes_len=%d\n", metrics.ProofPayloadBytesLen)

	// Verify if requested
	if doVerify {
		var vp verkle.VerkleProof
		if err := json.Unmarshal(result.VerkleProof, &vp); err != nil {
			return fmt.Errorf("unmarshal VerkleProof: %w", err)
		}
		var sd verkle.StateDiff
		if err := json.Unmarshal(result.StateDiff, &sd); err != nil {
			return fmt.Errorf("unmarshal StateDiff: %w", err)
		}

		verifyStart := time.Now()
		verifyErr := proof.VerifyProof(rootBytes, &vp, sd)
		bench.VerifyTime = time.Since(verifyStart)

		fmt.Fprintf(os.Stderr, "verify_time_ns=%d\n", bench.VerifyTime.Nanoseconds())
		if verifyErr != nil {
			fmt.Fprintf(os.Stderr, "verification=FAILED: %v\n", verifyErr)
		} else {
			fmt.Fprintf(os.Stderr, "verification=PASSED\n")
		}
	}

	// Print benchmark summary
	proof.PrintBenchResult(bench)

	return nil
}
