package main

import (
	"log"
	_ "net/http/pprof"
	"os"

	"github.com/urfave/cli/v2"
)

const defaultStartBlock = 18908895

func main() {
	app := &cli.App{
		Name:  "samuraimpt",
		Usage: "Samurai MPT",
		Commands: []*cli.Command{
			IngestCmd(),
			BuildMPTCmd(),
			benchIngestCmd(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
