package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nepal80m/samurai/internal/verklekzg/proof"
	"github.com/nepal80m/samurai/internal/verklekzg/store"
	"github.com/nepal80m/samurai/internal/verklekzg/tree"
	"github.com/urfave/cli/v2"
)

func getproofCmd() *cli.Command {
	return &cli.Command{
		Name:  "getproof",
		Usage: "Generate a Verkle-KZG balance proof for an address at a block",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Required: true, Usage: "Path to persistent DB directory"},
			&cli.Uint64Flag{Name: "block", Value: 0, Usage: "Block number"},
			&cli.StringFlag{Name: "address", Value: "", Usage: "Address (0x-prefixed hex)"},
			&cli.BoolFlag{Name: "verify", Value: false, Usage: "Also verify the proof"},
			&cli.BoolFlag{Name: "cold", Value: false, Usage: "Cold mode: reopen DB"},
			&cli.StringFlag{Name: "db-backend", Value: "pebble", Usage: "DB backend: pebble or leveldb"},
			&cli.StringFlag{Name: "params-dir", Value: "data/params/verklekzg", Usage: "Directory for precomputed SRS/barycentric files"},
		},
		Action: func(c *cli.Context) error {
			dbDir := c.String("db-dir")
			address := c.String("address")
			if address == "" {
				return fmt.Errorf("--address is required")
			}
			if !c.IsSet("block") {
				return fmt.Errorf("--block is required")
			}

			addrStr := strings.TrimPrefix(address, "0x")
			addrBytes, err := hex.DecodeString(addrStr)
			if err != nil || len(addrBytes) != 20 {
				return fmt.Errorf("invalid address: %s (must be 20-byte hex)", address)
			}
			var addr [20]byte
			copy(addr[:], addrBytes)

			return runGetProof(dbDir, c.String("db-backend"), c.String("params-dir"),
				c.Uint64("block"), addr, c.Bool("verify"), c.Bool("cold"))
		},
	}
}

func runGetProof(dbDir, dbBackend, paramsDir string, block uint64, addr [20]byte, doVerify, cold bool) error {
	treeCfg, err := tree.NewTreeConfig(paramsDir)
	if err != nil {
		return fmt.Errorf("init tree config: %w", err)
	}

	kv, err := store.OpenKVStore(dbBackend, dbDir)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	if cold {
		kv.Close()
		kv, err = store.OpenKVStore(dbBackend, dbDir)
		if err != nil {
			return fmt.Errorf("reopen db (cold): %w", err)
		}
	}
	defer kv.Close()

	ns := store.NewNodeStore(kv)
	resolver := ns.VersionedNodeResolverFn(block)

	root, err := ns.LoadTree(block)
	if err != nil {
		return fmt.Errorf("load tree for block %d: %w", block, err)
	}
	rootBytes := proof.SerializeCommitment(root)

	result, metrics, err := proof.GenerateProof(root, addr, rootBytes, resolver, treeCfg)
	if err != nil {
		return fmt.Errorf("generate proof: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("output JSON: %w", err)
	}

	fmt.Fprintf(os.Stderr, "proof_gen_time_ns=%d\n", metrics.ProofGenTimeNs)
	fmt.Fprintf(os.Stderr, "json_marshal_time_ns=%d\n", metrics.JSONMarshalTimeNs)
	fmt.Fprintf(os.Stderr, "proof_json_bytes_len=%d\n", metrics.ProofJSONBytesLen)
	fmt.Fprintf(os.Stderr, "proof_payload_bytes_len=%d\n", metrics.ProofPayloadBytesLen)

	if doVerify {
		var vkProof proof.VerkleKZGProof
		if err := json.Unmarshal(result.Proof, &vkProof); err != nil {
			return fmt.Errorf("unmarshal proof for verify: %w", err)
		}

		verifyStart := time.Now()
		verifyErr := proof.VerifyProof(rootBytes, &vkProof, treeCfg)
		verifyTime := time.Since(verifyStart)

		fmt.Fprintf(os.Stderr, "verify_time_ns=%d\n", verifyTime.Nanoseconds())
		if verifyErr != nil {
			fmt.Fprintf(os.Stderr, "verification=FAILED: %v\n", verifyErr)
		} else {
			fmt.Fprintf(os.Stderr, "verification=PASSED\n")
		}
	}

	return nil
}
