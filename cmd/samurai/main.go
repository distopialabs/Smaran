package main

import (
	"log"
	_ "net/http/pprof"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "samuraimpt",
		Usage: "Samurai MPT",
		Commands: []*cli.Command{
			IngestCmd(),
			BuildMPTCmd(),
			benchIngestCmd(),
			ProofCmd(),
			ServeCmd(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
