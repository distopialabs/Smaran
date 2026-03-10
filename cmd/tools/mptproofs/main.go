package main

import (
	"flag"

	"github.com/nepal80m/samurai/internal/logging"
)

var log = logging.GetLogger("mptproofs")

func main() {
	// read mode and call respective functions
	mode := flag.String("mode", "extract_proofs", "Mode to run: fetch_proofs, extract_proofs, verify_proofs")
	flag.Parse()

	switch *mode {
	case "fetch_proofs":
		fetchProofs()
	case "extract_proofs":
		extractProofs()
	case "verify_proofs":
		verifyProofs()
	}
}
