package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
)

const defaultProofDir = "/mydata/samurai/exp1/proofs"

type blockHeaderLite struct {
	StateRoot string `json:"stateRoot"`
}

type accountRLP struct {
	Nonce    uint64
	Balance  interface{}
	Root     common.Hash
	CodeHash []byte
}

type job struct {
	block uint64
}

type result struct {
	block       uint64
	ok          bool
	verifyDur   time.Duration
	endToEndDur time.Duration
	err         error
}

func verifyProofs() {
	var (
		accountArg    string
		startBlockArg uint64
		endBlockArg   uint64
		dirArg        string
		rpcArg        string
		concurrency   int
	)

	flag.StringVar(&accountArg, "account", "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", "account address")
	flag.Uint64Var(&startBlockArg, "start", 18908895, "start block (inclusive)")
	flag.Uint64Var(&endBlockArg, "end", 19108894, "end block (inclusive)")
	flag.StringVar(&dirArg, "dir", defaultProofDir, "directory containing .proof files")
	flag.StringVar(&rpcArg, "rpc", "/mydata/erigon/mainnet/erigon.ipc", "RPC endpoint (HTTP/WS/IPC)")
	flag.IntVar(&concurrency, "concurrency", 1, "number of concurrent verifications")
	flag.Parse()

	if accountArg == "" || endBlockArg < startBlockArg {
		fmt.Fprintln(os.Stderr, "usage: verify-proofs -account <addr> -start <n> -end <n> [-dir <proofDir>] [-rpc <endpoint>] [-concurrency <n>]")
		os.Exit(2)
	}
	if concurrency <= 0 {
		concurrency = 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cl, err := rpc.DialContext(ctx, rpcArg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial RPC:", err)
		os.Exit(1)
	}
	defer cl.Close()

	account := common.HexToAddress(accountArg)

	totalJobs := endBlockArg - startBlockArg + 1
	jobs := make(chan job, 2*concurrency)
	results := make(chan result, 2*concurrency)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			res := verifyOne(ctx, cl, account, dirArg, j.block)
			results <- res
		}
	}

	startWall := time.Now()
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}

	go func() {
		for b := startBlockArg; b <= endBlockArg; b++ {
			jobs <- job{block: b}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var (
		okCount, failCount   int
		minVerify, maxVerify time.Duration
		minE2E, maxE2E       time.Duration
		sumVerify, sumE2E    time.Duration
		processed            int
	)
	minVerify = time.Hour
	minE2E = time.Hour

	progressEvery := int(1000)
	if totalJobs < uint64(progressEvery) {
		progressEvery = int(totalJobs)
	}

	for res := range results {
		processed++
		if res.ok {
			okCount++
			sumVerify += res.verifyDur
			sumE2E += res.endToEndDur
			if res.verifyDur < minVerify {
				minVerify = res.verifyDur
			}
			if res.verifyDur > maxVerify {
				maxVerify = res.verifyDur
			}
			if res.endToEndDur < minE2E {
				minE2E = res.endToEndDur
			}
			if res.endToEndDur > maxE2E {
				maxE2E = res.endToEndDur
			}
		} else {
			failCount++
			fmt.Fprintf(os.Stderr, "block %d: %v\n", res.block, res.err)
		}
		if processed%progressEvery == 0 {
			fmt.Printf("progress: %d/%d verified (ok=%d fail=%d)\n", processed, totalJobs, okCount, failCount)
		}
	}

	totalWall := time.Since(startWall)
	avgVerify := time.Duration(0)
	avgE2E := time.Duration(0)
	if okCount > 0 {
		avgVerify = time.Duration(int64(sumVerify) / int64(okCount))
		avgE2E = time.Duration(int64(sumE2E) / int64(okCount))
	}

	fmt.Println("================ Summary ================")
	fmt.Printf("Account:      %s\n", account.Hex())
	fmt.Printf("Blocks:       %d - %d (%d total)\n", startBlockArg, endBlockArg, totalJobs)
	fmt.Printf("Concurrency:  %d (CPUs: %d)\n", concurrency, runtime.NumCPU())
	fmt.Printf("Verified OK:  %d\n", okCount)
	fmt.Printf("Failed:       %d\n", failCount)
	fmt.Printf("Total time:   %s (%.2f proofs/sec)\n", totalWall, float64(okCount)/totalWall.Seconds())
	fmt.Printf("Verify time:  min=%s max=%s avg=%s\n", minVerify, maxVerify, avgVerify)
	fmt.Printf("End-to-end:   min=%s max=%s avg=%s\n", minE2E, maxE2E, avgE2E)
}

func verifyOne(ctx context.Context, cl *rpc.Client, account common.Address, proofDir string, block uint64) result {
	e2eStart := time.Now()

	proofPath, err := findProofPath(proofDir, account.Hex(), int64(block))
	if err != nil {
		return result{block: block, ok: false, err: err}
	}
	nodes, err := readProofFile(proofPath)
	if err != nil {
		return result{block: block, ok: false, err: fmt.Errorf("read proof: %w", err)}
	}

	var hdr blockHeaderLite
	blockTag := fmt.Sprintf("0x%x", block)
	if err := cl.CallContext(ctx, &hdr, "eth_getBlockByNumber", blockTag, false); err != nil {
		return result{block: block, ok: false, err: fmt.Errorf("get header: %w", err)}
	}
	stateRoot := common.HexToHash(hdr.StateRoot)

	verifyStart := time.Now()
	if err := verifyNodes(stateRoot, account, nodes); err != nil {
		return result{block: block, ok: false, verifyDur: time.Since(verifyStart), endToEndDur: time.Since(e2eStart), err: fmt.Errorf("verify: %w", err)}
	}

	return result{block: block, ok: true, verifyDur: time.Since(verifyStart), endToEndDur: time.Since(e2eStart)}
}

func verifyNodes(stateRoot common.Hash, account common.Address, nodes [][]byte) error {
	pdb := memorydb.New()
	for _, b := range nodes {
		h := crypto.Keccak256(b)
		if err := pdb.Put(h, b); err != nil {
			return err
		}
	}
	key := crypto.Keccak256(account.Bytes())
	leaf, err := trie.VerifyProof(stateRoot, key, pdb)
	if err != nil {
		return err
	}
	// Best-effort RLP decoding as a sanity check (ignore contents)
	var acc accountRLP
	_ = rlp.DecodeBytes(leaf, &acc)
	return nil
}

func findProofPath(dir, account string, block int64) (string, error) {
	p := filepath.Join(dir, fmt.Sprintf("%s_%d.proof", account, block))
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("proof file not found for account %q block %d in %q", account, block, dir)
}

func readProofFile(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var nodes [][]byte
	for {
		var l uint32
		if err := binary.Read(f, binary.BigEndian, &l); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("truncated length in %s: %w", path, err)
			}
			return nil, err
		}
		if l == 0 {
			nodes = append(nodes, []byte{})
			continue
		}
		buf := make([]byte, l)
		if _, err := io.ReadFull(f, buf); err != nil {
			return nil, fmt.Errorf("truncated payload in %s: %w", path, err)
		}
		nodes = append(nodes, buf)
	}
	return nodes, nil
}
