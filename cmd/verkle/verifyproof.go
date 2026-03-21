package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nepal80m/samurai/internal/verkle/proof"
	verkle "github.com/ethereum/go-verkle"
	"github.com/urfave/cli/v2"
)

func verifyproofCmd() *cli.Command {
	return &cli.Command{
		Name:  "verifyproof",
		Usage: "Verify a Verkle balance proof from JSON",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "root", Value: "", Usage: "State root (0x-prefixed hex, required)"},
			&cli.StringFlag{Name: "address", Value: "", Usage: "Address (0x-prefixed hex, required)"},
			&cli.StringFlag{Name: "proof", Value: "-", Usage: "Path to proof JSON file (or - for stdin)"},
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

			// Parse root
			rootBytes, err := hex.DecodeString(strings.TrimPrefix(rootStr, "0x"))
			if err != nil {
				return fmt.Errorf("invalid root: %w", err)
			}

			// Parse address
			addrBytes, err := hex.DecodeString(strings.TrimPrefix(address, "0x"))
			if err != nil || len(addrBytes) != 20 {
				return fmt.Errorf("invalid address: %s", address)
			}
			var addr [20]byte
			copy(addr[:], addrBytes)

			// Read proof JSON
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

			return runVerifyProof(rootBytes, addr, proofJSON)
		},
	}
}

func runVerifyProof(rootBytes []byte, addr [20]byte, proofJSON []byte) error {
	// Parse wrapper JSON
	var wrapper proof.ProofResult
	if err := json.Unmarshal(proofJSON, &wrapper); err != nil {
		return fmt.Errorf("parse proof JSON: %w", err)
	}

	// Extract VerkleProof and StateDiff
	var vp verkle.VerkleProof
	if err := json.Unmarshal(wrapper.VerkleProof, &vp); err != nil {
		return fmt.Errorf("parse verkleProof: %w", err)
	}
	var sd verkle.StateDiff
	if err := json.Unmarshal(wrapper.StateDiff, &sd); err != nil {
		return fmt.Errorf("parse stateDiff: %w", err)
	}

	// Verify
	verifyStart := time.Now()
	exists, balance, err := proof.VerifyAndExtract(rootBytes, &vp, sd, addr)
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
