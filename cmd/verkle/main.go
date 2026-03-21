package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "verkle",
		Short: "Baseline Verkle tree proof benchmarking tool",
		Long: `A CLI tool for ingesting Ethereum block data into a Verkle tree,
generating EIP-6800 balance proofs, and benchmarking proof performance.`,
	}

	rootCmd.AddCommand(ingestCmd())
	rootCmd.AddCommand(benchIngestCmd())
	rootCmd.AddCommand(getproofCmd())
	rootCmd.AddCommand(verifyproofCmd())
	rootCmd.AddCommand(serveCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
