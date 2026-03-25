package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "verklekzg",
		Usage: "Verkle-KZG tree proof benchmarking tool (BLS12-381 + KZG commitments)",
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
