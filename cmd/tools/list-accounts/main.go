package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

// debug_accountRange response shape (approximate; handles key or value-address)
type accountRangeResp struct {
	Accounts map[string]struct {
		Address string `json:"address"`
	} `json:"accounts"`
	Next string `json:"next"`
}

func main() {
	ipc := flag.String("ipc", "/mydata/erigon/mainnet/erigon.ipc", "Erigon IPC path")
	block := flag.Uint64("block", 18908895, "Block number to enumerate state at")
	limit := flag.Int("limit", 1000000, "Maximum accounts to write")
	batch := flag.Int("batch", 10000, "Accounts to fetch per RPC call")
	// start := flag.String("start", "0x0000000000000000000000000000000000000000", "Start key/address (inclusive)")
	out := flag.String("out", "accounts_18908895.txt", "Output file path")

	flag.Parse()

	ctx := context.Background()
	rpcClient, err := rpc.DialContext(ctx, *ipc)
	if err != nil {
		log.Fatalf("failed to connect to IPC: %v", err)
	}
	defer rpcClient.Close()

	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	defer w.Flush()

	// cursor := *start
	cursor := []byte{}
	written := 0
	// bhex := fmt.Sprintf("0x%x", *block)
	for written < *limit {
		fmt.Println("Making RPC call")
		var resp accountRangeResp
		if err := rpcClient.CallContext(ctx, &resp, "debug_accountRange", *block, cursor, *batch, true, true); err != nil {
			log.Fatalf("debug_accountRange failed: %v", err)
		}
		fmt.Println("RPC call done")
		if len(resp.Accounts) == 0 {
			break
		}
		for k, v := range resp.Accounts {
			addr := v.Address
			if addr == "" {
				addr = k
			}
			addr = strings.TrimSpace(addr)
			if !strings.HasPrefix(addr, "0x") {
				addr = "0x" + addr
			}
			if !common.IsHexAddress(addr) {
				continue
			}
			if _, err := w.WriteString(strings.ToLower(addr) + "\n"); err != nil {
				log.Fatalf("failed writing address: %v", err)
			}
			written++
			if written >= *limit {
				break
			}
		}
		if resp.Next == "" || resp.Next == string(cursor) {
			break
		}
		cursor = []byte(resp.Next)
		fmt.Println(resp.Next)
		if written%1000 == 0 {
			log.Printf("listed %d accounts...", written)
		}
	}
	log.Printf("done. wrote %d accounts to %s", written, *out)
}
