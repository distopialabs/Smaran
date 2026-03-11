// Binary ktserver runs a Key Transparency HTTP/JSON server.
//
// Usage:
//
//	ktserver --addr 0.0.0.0:3191 --protocol optiks
//
// Endpoints:
//
//	POST /put            – Put(user, key)
//	POST /get            – Get(user)
//	POST /get_commitment – GetCommitment()
//
// See KT.md for the full protocol specification.
package main

import (
	"flag"
	"net/http"

	"github.com/nepal80m/samurai/internal/kt"
	"github.com/nepal80m/samurai/internal/logging"
)

var log = logging.GetLogger("ktserver")

func main() {
	addr := flag.String("addr", "0.0.0.0:3191", "IP:port to bind to")
	protocol := flag.String("protocol", "optiks", "protocol to use: 'samurai' or 'optiks'")
	flag.Parse()

	p := kt.Protocol(*protocol)
	if p != kt.ProtocolOptiks && p != kt.ProtocolSamurai {
		log.Fatalf("invalid --protocol %q: must be 'samurai' or 'optiks'", *protocol)
	}

	handler := kt.NewKTHandler(p)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	log.Infof("Starting KT server on %s with protocol %s", *addr, *protocol)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
