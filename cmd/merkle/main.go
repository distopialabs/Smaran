package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/urfave/cli/v2"

	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/merkle/ingest"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	"github.com/nepal80m/samurai/internal/merkle/proof"
	"github.com/nepal80m/samurai/internal/merkle/server"
	st "github.com/nepal80m/samurai/internal/merkle/state"
)

const defaultStartBlock = 18908895

func main() {
	app := &cli.App{
		Name:  "merkle",
		Usage: "Baseline Merkle Patricia Trie proof benchmarking tool",
		Commands: []*cli.Command{
			ingestCmd(),
			benchIngestCmd(),
			getProofCmd(),
			verifyProofCmd(),
			serveCmd(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// ── ingest ──────────────────────────────────────────────────────────

func ingestCmd() *cli.Command {
	return &cli.Command{
		Name:  "ingest",
		Usage: "Ingest block data into the Merkle Patricia Trie",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Value: "/data/local/tmp/merkle", Usage: "Path to state database directory"},
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to blocks data directory"},
			&cli.Uint64Flag{Name: "n", Value: 1000, Usage: "Number of blocks to ingest"},
			&cli.BoolFlag{Name: "fresh", Value: false, Usage: "Delete existing DB and start from scratch"},
		},
		Action: func(c *cli.Context) error {
			dbDir := c.String("db-dir")

			startBlock := dataset.FIRST_BLOCK
			endBlock := startBlock + c.Uint64("n") - 1

			// If --fresh, remove existing database directory before opening.
			if c.Bool("fresh") {
				if _, err := os.Stat(dbDir); err == nil {
					log.Printf("--fresh: removing existing database at %s", dbDir)
					if err := os.RemoveAll(dbDir); err != nil {
						return fmt.Errorf("--fresh: failed to remove %s: %w", dbDir, err)
					}
				}
			}

			store, err := st.OpenDB(dbDir)
			if err != nil {
				return err
			}
			defer store.Close()

			cfg := ingest.Config{
				BlocksDir: c.String("blocks-dir"),
				Store:     store,
				Start:     startBlock,
				End:       endBlock,
			}
			return ingest.Run(cfg)
		},
	}
}

// ── bench-ingest ────────────────────────────────────────────────────

func benchIngestCmd() *cli.Command {
	return &cli.Command{
		Name:  "bench-ingest",
		Usage: "Benchmark block ingestion for a fixed duration, write per-block timing CSV",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to blocks data directory"},
			&cli.StringFlag{Name: "db-dir", Value: "/data/local/tmp/bench-merkle", Usage: "Path to state database directory"},
			&cli.DurationFlag{Name: "duration", Value: 5 * time.Minute, Usage: "How long to run the benchmark"},
			&cli.IntFlag{Name: "k-users", Value: 0, Usage: "Top-K hot accounts to include (0 = all, no filtering)"},
			&cli.StringFlag{Name: "accounts-list", Value: "account_stats_all.csv", Usage: "CSV with hot accounts sorted by update count descending"},
			&cli.IntFlag{Name: "num-shards", Value: 1, Usage: "Number of shard workers in the pipeline"},
			&cli.BoolFlag{Name: "fresh", Value: true, Usage: "Delete existing DB and start from scratch"},
			&cli.StringFlag{Name: "output-dir", Value: benchutil.DefaultOutputDir, Usage: "Root directory for benchmark output"},
		},
		Action: func(c *cli.Context) error {
			dbDir := c.String("db-dir")
			kUsers := c.Int("k-users")
			outputDir := c.String("output-dir")

			if c.Bool("fresh") {
				if _, err := os.Stat(dbDir); err == nil {
					log.Printf("--fresh: removing existing database at %s", dbDir)
					if err := os.RemoveAll(dbDir); err != nil {
						return fmt.Errorf("--fresh: failed to remove %s: %w", dbDir, err)
					}
				}
			}

			csvPath, err := benchutil.IngestionOutputPath(outputDir, "merkle", kUsers)
			if err != nil {
				return err
			}
			updateMetricsPath, err := benchutil.UpdateMetricsOutputPath(outputDir, "merkle", kUsers)
			if err != nil {
				return err
			}

			store, err := st.OpenDB(dbDir)
			if err != nil {
				return err
			}
			defer store.Close()

			cfg := ingest.BenchConfig{
				BlocksDir:         c.String("blocks-dir"),
				Store:             store,
				Start:             dataset.FIRST_BLOCK,
				Duration:          c.Duration("duration"),
				KUsers:            kUsers,
				NumShards:         c.Int("num-shards"),
				AccountsList:      c.String("accounts-list"),
				OutCSV:            csvPath,
				UpdateMetricsPath: updateMetricsPath,
			}
			return ingest.BenchRun(cfg)
		},
	}
}

// ── getproof ────────────────────────────────────────────────────────

func getProofCmd() *cli.Command {
	return &cli.Command{
		Name:  "getproof",
		Usage: "Generate an eth_getProof-style account proof",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Required: true, Usage: "Path to state database directory"},
			&cli.StringFlag{Name: "db-backend", Value: "pebble", Usage: "Database backend: pebble or leveldb"},
			&cli.Uint64Flag{Name: "block", Required: true, Usage: "Block number to query"},
			&cli.StringFlag{Name: "address", Required: true, Usage: "Account address (0x hex)"},
			&cli.BoolFlag{Name: "verify", Value: true, Usage: "Verify proof after generation"},
			&cli.BoolFlag{Name: "cold", Value: false, Usage: "Cold mode: reopen DB to simulate cold reads"},
		},
		Action: func(c *cli.Context) error {
			blockNum := c.Uint64("block")
			addrStr := c.String("address")
			addr := common.HexToAddress(addrStr)
			cold := c.Bool("cold")
			doVerify := c.Bool("verify")
			dbDir := c.String("db-dir")
			dbBackend := c.String("db-backend")

			// Open DB.
			store, err := st.OpenDBWithBackend(dbDir, dbBackend)
			if err != nil {
				return err
			}

			// Cold mode: close and reopen to clear caches.
			if cold {
				store.Close()
				store, err = st.OpenDBWithBackend(dbDir, dbBackend)
				if err != nil {
					return err
				}
			}
			defer store.Close()

			// Load root.
			root, err := meta.GetRoot(store.DiskDB, blockNum)
			if err != nil {
				return fmt.Errorf("no root for block %d: %w", blockNum, err)
			}

			// Open state.
			stateDB, err := store.OpenState(root)
			if err != nil {
				return err
			}

			// Get the trie for proof generation.
			stateTrie := stateDB.GetTrie()

			// Generate proof.
			genStart := time.Now()
			result, rawNodes, err := proof.GenerateAccountProof(stateDB, root, addr, stateTrie)
			proofGenTime := time.Since(genStart)
			if err != nil {
				return fmt.Errorf("generate proof: %w", err)
			}

			var proofByteSize int
			for _, n := range rawNodes {
				proofByteSize += len(n)
			}

			// Marshal JSON.
			jsonBytes, err := proof.MarshalJSON(result)
			if err != nil {
				return fmt.Errorf("marshal JSON: %w", err)
			}

			// Verify if requested.
			var verifyTime time.Duration
			if doVerify {
				verifyStart := time.Now()
				exists, bal, err := proof.VerifyAccountProof(root, addr, rawNodes)
				verifyTime = time.Since(verifyStart)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Verification FAILED: %v\n", err)
				} else if exists {
					fmt.Fprintf(os.Stderr, "Verification PASSED (balance: %s)\n", bal.String())
				} else {
					fmt.Fprintf(os.Stderr, "Verification PASSED (account does not exist)\n")
				}
			}

			// Print metrics.
			fmt.Fprintf(os.Stderr, "Proof gen:   %v\n", proofGenTime)
			fmt.Fprintf(os.Stderr, "Proof bytes: %d (%d nodes)\n", proofByteSize, len(rawNodes))
			fmt.Fprintf(os.Stderr, "JSON size:   %d\n", len(jsonBytes))
			if doVerify {
				fmt.Fprintf(os.Stderr, "Verify time: %v\n", verifyTime)
			}

			// Print JSON to stdout.
			fmt.Println(string(jsonBytes))
			return nil
		},
	}
}

// ── verifyproof ─────────────────────────────────────────────────────

func verifyProofCmd() *cli.Command {
	return &cli.Command{
		Name:  "verifyproof",
		Usage: "Verify an account proof offline from JSON",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "root", Required: true, Usage: "State root hash (0x hex)"},
			&cli.StringFlag{Name: "address", Required: true, Usage: "Account address (0x hex)"},
			&cli.StringFlag{Name: "proof", Value: "-", Usage: "Proof JSON file (or - for stdin)"},
		},
		Action: func(c *cli.Context) error {
			rootHash := common.HexToHash(c.String("root"))
			addr := common.HexToAddress(c.String("address"))
			proofFile := c.String("proof")

			// Read proof JSON.
			var data []byte
			var err error
			if proofFile == "-" {
				data, err = io.ReadAll(os.Stdin)
			} else {
				data, err = os.ReadFile(proofFile)
			}
			if err != nil {
				return fmt.Errorf("read proof: %w", err)
			}

			// Parse JSON.
			var proofResult struct {
				AccountProof []string `json:"accountProof"`
			}
			if err := json.Unmarshal(data, &proofResult); err != nil {
				return fmt.Errorf("parse proof JSON: %w", err)
			}

			// Decode hex proof nodes.
			nodes := make([][]byte, len(proofResult.AccountProof))
			for i, s := range proofResult.AccountProof {
				s = strings.TrimPrefix(s, "0x")
				b, err := hex.DecodeString(s)
				if err != nil {
					return fmt.Errorf("decode proof node %d: %w", i, err)
				}
				nodes[i] = b
			}

			// Build proof DB.
			secureKey := crypto.Keccak256(addr.Bytes())
			proofDB := memorydb.New()
			for _, node := range nodes {
				key := crypto.Keccak256(node)
				proofDB.Put(key, node)
			}

			// Verify.
			verifyStart := time.Now()
			val, err := trie.VerifyProof(rootHash, secureKey, proofDB)
			verifyTime := time.Since(verifyStart)

			if err != nil {
				fmt.Printf("FAILED: %v\n", err)
				fmt.Fprintf(os.Stderr, "Verification time: %s\n", verifyTime.Round(time.Microsecond))
				return err
			}

			if val == nil {
				fmt.Println("VERIFIED (account does not exist)")
				fmt.Printf("Balance: 0\n")
			} else {
				// Decode RLP account.
				var acct struct {
					Nonce       uint64
					Balance     *big.Int
					StorageRoot common.Hash
					CodeHash    []byte
				}
				if err := rlp.DecodeBytes(val, &acct); err != nil {
					return fmt.Errorf("RLP decode: %w", err)
				}
				fmt.Println("VERIFIED")
				fmt.Printf("Balance: %s (0x%s)\n", acct.Balance.String(), hexutil.EncodeBig(acct.Balance))
				fmt.Printf("Nonce  : %d\n", acct.Nonce)
				fmt.Printf("Storage: %s\n", common.BytesToHash(acct.StorageRoot[:]).Hex())
				fmt.Printf("Code   : %s\n", common.BytesToHash(acct.CodeHash).Hex())
			}
			fmt.Fprintf(os.Stderr, "Verification time: %s\n", verifyTime.Round(time.Microsecond))
			return nil
		},
	}
}

// ── serve ───────────────────────────────────────────────────────────

func serveCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the gRPC range proof server",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Required: true, Usage: "Path to state database directory"},
			&cli.StringFlag{Name: "host", Value: "0.0.0.0", Usage: "gRPC server host"},
			&cli.IntFlag{Name: "port", Value: 50051, Usage: "gRPC server port"},
		},
		Action: func(c *cli.Context) error {
			host := c.String("host")
			port := c.Int("port")
			addr := fmt.Sprintf("%s:%d", host, port)
			store, err := st.OpenDB(c.String("db-dir"))
			if err != nil {
				return err
			}
			defer store.Close()

			proofServer := server.NewProofServer(store)
			return server.ListenAndServe(addr, proofServer)
		},
	}
}
