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
	"github.com/spf13/cobra"
)

func verifyproofCmd() *cobra.Command {
	var (
		root      string
		address   string
		proofFile string
	)

	cmd := &cobra.Command{
		Use:   "verifyproof",
		Short: "Verify a Verkle balance proof from JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse root
			rootStr := strings.TrimPrefix(root, "0x")
			rootBytes, err := hex.DecodeString(rootStr)
			if err != nil {
				return fmt.Errorf("invalid root: %w", err)
			}

			// Parse address
			addrStr := strings.TrimPrefix(address, "0x")
			addrBytes, err := hex.DecodeString(addrStr)
			if err != nil || len(addrBytes) != 20 {
				return fmt.Errorf("invalid address: %s", address)
			}
			var addr [20]byte
			copy(addr[:], addrBytes)

			// Read proof JSON
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

	cmd.Flags().StringVar(&root, "root", "", "State root (0x-prefixed hex)")
	cmd.Flags().StringVar(&address, "address", "", "Address (0x-prefixed hex)")
	cmd.Flags().StringVar(&proofFile, "proof", "-", "Path to proof JSON file (or - for stdin)")
	cmd.MarkFlagRequired("root")
	cmd.MarkFlagRequired("address")

	return cmd
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
