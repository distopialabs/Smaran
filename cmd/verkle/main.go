package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "verkle",
		Usage: "Baseline Verkle tree proof benchmarking tool",
		Commands: []*cli.Command{
			ingestCmd(),
			benchIngestCmd(),
			getproofCmd(),
			verifyproofCmd(),
			serveCmd(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
