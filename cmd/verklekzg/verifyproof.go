package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nepal80m/samurai/internal/verklekzg/proof"
	"github.com/nepal80m/samurai/internal/verklekzg/tree"
	"github.com/urfave/cli/v2"
)

func verifyproofCmd() *cli.Command {
	return &cli.Command{
		Name:  "verifyproof",
		Usage: "Verify a Verkle-KZG balance proof from JSON",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "root", Value: "", Usage: "State root (0x-prefixed hex)"},
			&cli.StringFlag{Name: "address", Value: "", Usage: "Address (0x-prefixed hex)"},
			&cli.StringFlag{Name: "proof", Value: "-", Usage: "Path to proof JSON file (or - for stdin)"},
			&cli.StringFlag{Name: "params-dir", Value: "data/params/verklekzg", Usage: "Directory for precomputed SRS/barycentric files"},
		},
		Action: func(c *cli.Context) error {
			rootStr := c.String("root")
			if rootStr == "" {
				return fmt.Errorf("--root is required")
			}
			address := c.String("address")
			if address == "" {
				return fmt.Errorf("--address is required")
			}

			rootBytes, err := hex.DecodeString(strings.TrimPrefix(rootStr, "0x"))
			if err != nil {
				return fmt.Errorf("invalid root: %w", err)
			}

			addrBytes, err := hex.DecodeString(strings.TrimPrefix(address, "0x"))
			if err != nil || len(addrBytes) != 20 {
				return fmt.Errorf("invalid address: %s", address)
			}
			var addr [20]byte
			copy(addr[:], addrBytes)

			proofFile := c.String("proof")
			var proofJSON []byte
			if proofFile == "" || proofFile == "-" {
				proofJSON, err = io.ReadAll(os.Stdin)
			} else {
				proofJSON, err = os.ReadFile(proofFile)
			}
			if err != nil {
				return fmt.Errorf("read proof: %w", err)
			}

			return runVerifyProof(rootBytes, addr, proofJSON, c.String("params-dir"))
		},
	}
}

func runVerifyProof(rootBytes []byte, addr [20]byte, proofJSON []byte, paramsDir string) error {
	treeCfg, err := tree.NewTreeConfig(paramsDir)
	if err != nil {
		return fmt.Errorf("init tree config: %w", err)
	}

	var wrapper proof.ProofResult
	if err := json.Unmarshal(proofJSON, &wrapper); err != nil {
		return fmt.Errorf("parse proof JSON: %w", err)
	}

	var vkProof proof.VerkleKZGProof
	if err := json.Unmarshal(wrapper.Proof, &vkProof); err != nil {
		return fmt.Errorf("parse VerkleKZGProof: %w", err)
	}

	verifyStart := time.Now()
	exists, balance, err := proof.VerifyAndExtract(rootBytes, &vkProof, addr, treeCfg)
	verifyTime := time.Since(verifyStart)

	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		fmt.Fprintf(os.Stderr, "verify_time_ns=%d\n", verifyTime.Nanoseconds())
		return err
	}

	fmt.Println("VERIFIED")
	fmt.Printf("exists=%v\n", exists)
	fmt.Printf("balance=0x%s\n", balance.Text(16))
	fmt.Fprintf(os.Stderr, "verify_time_ns=%d\n", verifyTime.Nanoseconds())

	return nil
}
