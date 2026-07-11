// Package main provides a CLI client for the Samurai gRPC proof service.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gogo/protobuf/proto"
	"github.com/urfave/cli/v2"

	proofpb "github.com/nepal80m/samurai/api/proto/samurai/v1"
	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/server"
	"github.com/nepal80m/samurai/internal/tree"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const maxMsgSize = 100 * 1024 * 1024 // 100MB

func main() {
	app := &cli.App{
		Name:  "proofc",
		Usage: "Samurai gRPC proof client",
		Commands: []*cli.Command{
			queryCmd(),
			benchCmd(),
			openloopCmd(),
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// --- query subcommand ---

func queryCmd() *cli.Command {
	return &cli.Command{
		Name:  "query",
		Usage: "Query a single range proof and verify it",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "server-addr", Value: "localhost:50051", Usage: "gRPC server address"},
			&cli.StringFlag{Name: "account", Value: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", Usage: "Account address (0x hex)"},
			&cli.Uint64Flag{Name: "start-block", Value: dataset.FIRST_BLOCK, Usage: "Start block number"},
			&cli.Uint64Flag{Name: "end-block", Value: dataset.FIRST_BLOCK + 1000 - 1, Usage: "End block number"},
			&cli.StringFlag{Name: "params-dir", Value: "./data/params", Usage: "Path to crypto params"},
			&cli.StringFlag{Name: "state-root", Value: "", Usage: "MPT state root hash (hex) for verification"},
			&cli.BoolFlag{Name: "old", Value: false, Usage: "Use old (slow) proof generation"},
		},
		Action: func(c *cli.Context) error {
			conn, err := dialGRPC(c.String("server-addr"))
			if err != nil {
				return err
			}
			defer conn.Close()
			client := proofpb.NewProofServiceClient(conn)

			precomputed, err := setupPrecomputedData(c.String("params-dir"))
			if err != nil {
				return err
			}

			stateRoot := common.Hash{}
			if s := c.String("state-root"); s != "" {
				stateRoot = common.HexToHash(s)
			}

			account := c.String("account")
			startBlock := c.Uint64("start-block")
			endBlock := c.Uint64("end-block")

			req := &proofpb.GetProofRequest{
				Account:    account,
				StartBlock: startBlock,
				EndBlock:   endBlock,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			respStart := time.Now()
			resp, fetchErr := fetchProofStream(ctx, client, req, c.Bool("old"))
			respDur := time.Since(respStart)

			if fetchErr != nil {
				s := samuraiQuerySummary{
					Account:    account,
					StartBlock: startBlock,
					EndBlock:   endBlock,
					ResponseDur:     respDur,
				}
				if isClientError(fetchErr) {
					s.ClientErr = fetchErr.Error()
				} else {
					s.ServerErr = fetchErr.Error()
				}
				printSamuraiQuerySummary(os.Stderr, s)
				return fmt.Errorf("GetProofStream failed: %w", fetchErr)
			}

			// Extract version range from balance infos.
			var startVersion uint64
			var endVersion uint64
			if len(resp.BalanceInfos) > 0 {
				startVersion = math.MaxUint64
				for _, bi := range resp.BalanceInfos {
					if bi.Version < startVersion {
						startVersion = bi.Version
					}
					if bi.Version > endVersion {
						endVersion = bi.Version
					}
				}
			}

			payloadBytes := int64(proto.Size(resp))

			// Verify.
			addr := common.HexToAddress(account)
			verifyStart := time.Now()
			verifyErr := verifyResponse(resp, addr, precomputed, stateRoot)
			verifyDur := time.Since(verifyStart)

			printSamuraiQuerySummary(os.Stderr, samuraiQuerySummary{
				Account:      account,
				StartBlock:   startBlock,
				EndBlock:     endBlock,
				StartVersion: startVersion,
				EndVersion:   endVersion,
				ProofgenDur:  time.Duration(resp.ProofgenDurationNs),
				ResponseDur:       respDur,
				VerifyDur:    verifyDur,
				Verified:     true,
				VerifyOK:     verifyErr == nil,
				PayloadBytes: payloadBytes,
				RangeProofs:  len(resp.RangeProofs),
				BalanceInfos: len(resp.BalanceInfos),
			})

			return verifyErr
		},
	}
}

// --- bench subcommand ---

func benchCmd() *cli.Command {
	return &cli.Command{
		Name:  "bench",
		Usage: "Run a proof generation benchmark",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "server-addr", Value: "localhost:50051", Usage: "gRPC server address"},
			&cli.IntFlag{Name: "range-size", Value: 50000, Usage: "Block range size per query"},
			&cli.IntFlag{Name: "num-clients", Value: 1, Usage: "Number of concurrent client goroutines"},
			&cli.StringFlag{Name: "accounts-list", Required: true, Usage: "CSV with accounts sorted by update count desc"},
			&cli.DurationFlag{Name: "duration", Value: 60 * time.Second, Usage: "Benchmark duration"},
			&cli.BoolFlag{Name: "verify", Value: true, Usage: "Verify proofs (requires --params-dir)"},
			&cli.StringFlag{Name: "params-dir", Value: "./data/params", Usage: "Path to crypto params (for verification)"},
			&cli.StringFlag{Name: "state-root", Value: "", Usage: "MPT state root hash (hex) for verification"},
			&cli.BoolFlag{Name: "old", Value: false, Usage: "Use old (slow) proof generation"},
			&cli.StringFlag{Name: "output-dir", Value: benchutil.DefaultOutputDir, Usage: "Root directory for benchmark output"},
		},
		Action: func(c *cli.Context) error {
			conn, err := dialGRPC(c.String("server-addr"))
			if err != nil {
				return err
			}
			defer conn.Close()
			client := proofpb.NewProofServiceClient(conn)

			useOld := c.Bool("old")

			// Call GetInfo to get latest block.
			info, err := client.GetInfo(context.Background(), &proofpb.GetInfoRequest{})
			if err != nil {
				return fmt.Errorf("GetInfo failed: %w", err)
			}
			log.Printf("Server info: latest_block=%d state_root=%s", info.LatestBlock, info.StateRoot)

			rangeSize := c.Int("range-size")
			firstBlock := dataset.FIRST_BLOCK
			if info.LatestBlock-firstBlock+1 < uint64(rangeSize) {
				return fmt.Errorf("server has %d blocks, need at least %d for range-size=%d",
					info.LatestBlock-firstBlock+1, rangeSize, rangeSize)
			}

			selector, err := benchutil.NewWeightedAccountSelector(c.String("accounts-list"))
			if err != nil {
				return err
			}
			log.Printf("Loaded %d weighted accounts", selector.Size())

			// Setup verification if requested.
			var precomputed *config.PrecomputedData
			stateRoot := common.Hash{}
			if c.Bool("verify") {
				precomputed, err = setupPrecomputedData(c.String("params-dir"))
				if err != nil {
					return err
				}
				if s := c.String("state-root"); s != "" {
					stateRoot = common.HexToHash(s)
				}
			}

			cfg := benchutil.ProofBenchConfig{
				ServerAddr: c.String("server-addr"),
				RangeSize:  rangeSize,
				NumClients: c.Int("num-clients"),
				Duration:   c.Duration("duration"),
				FirstBlock: firstBlock,
			}

			// Run benchmark.
			allStats := make([]benchutil.ClientStats, cfg.NumClients)
			var wg sync.WaitGroup
			deadline := time.Now().Add(cfg.Duration)
			benchStart := time.Now()

			for i := 0; i < cfg.NumClients; i++ {
				wg.Add(1)
				go func(clientID int) {
					defer wg.Done()

					// Each goroutine gets its own connection for realistic load.
					clientConn, err := dialGRPC(cfg.ServerAddr)
					if err != nil {
						log.Printf("client %d: dial failed: %v", clientID, err)
						return
					}
					defer clientConn.Close()
					cl := proofpb.NewProofServiceClient(clientConn)

					var stats benchutil.ClientStats

					for time.Now().Before(deadline) {
						account := selector.Pick()
						endBlock := info.LatestBlock
						startBlock := endBlock - uint64(rangeSize) + 1

						req := &proofpb.GetProofRequest{
							Account:    account,
							StartBlock: startBlock,
							EndBlock:   endBlock,
						}

						respStart := time.Now()
						resp, reqErr := fetchProofStream(context.Background(), cl, req, useOld)
						respNs := time.Since(respStart).Nanoseconds()

						if reqErr != nil {
							if isClientError(reqErr) {
								log.Printf("client error: %v", reqErr)
								stats.TotalClientErrors++
							} else {
								stats.TotalServerErrors++
								if stats.TotalServerErrors <= 3 {
									log.Printf("client %d: server error: %v", clientID, reqErr)
								}
							}
							continue
						}

						stats.TotalRequests++
						stats.TotalResponseNs += respNs
						stats.TotalProofgenNs += resp.ProofgenDurationNs
						stats.TotalPayloadBytes += int64(proto.Size(resp))

						if precomputed != nil {
							addr := common.HexToAddress(account)
							vStart := time.Now()
							if err := verifyResponse(resp, addr, precomputed, stateRoot); err != nil {
								stats.TotalVerifyFailures++
							}
							stats.TotalVerifyNs += time.Since(vStart).Nanoseconds()
						}
					}

					allStats[clientID] = stats
				}(i)
			}

			wg.Wait()
			wallDuration := time.Since(benchStart)

			agg := benchutil.AggregateStats(allStats)
			benchutil.PrintSummary(os.Stdout, cfg, agg, wallDuration)

			if err := benchutil.WriteSummaryFile(c.String("output-dir"), "samuraimpt", cfg, agg, wallDuration); err != nil {
				log.Printf("warning: failed to write summary file: %v", err)
			}

			return nil
		},
	}
}

// --- openloop subcommand ---

func openloopCmd() *cli.Command {
	return &cli.Command{
		Name:  "openloop",
		Usage: "Open-loop throughput test: fire requests at a fixed rate to find max server throughput",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "server-addr", Value: "localhost:50051", Usage: "gRPC server address"},
			&cli.IntFlag{Name: "range-size", Value: 50000, Usage: "Block range size per query"},
			&cli.StringFlag{Name: "accounts-list", Required: true, Usage: "CSV with accounts sorted by update count desc"},
			&cli.IntFlag{Name: "num-clients", Value: 1, Usage: "Number of concurrent client connections"},
			&cli.IntFlag{Name: "rps-per-client", Value: 10, Usage: "Requests per second per client connection"},
			&cli.IntFlag{Name: "max-concurrent", Value: 100, Usage: "Max in-flight requests per connection (semaphore size)"},
			&cli.DurationFlag{Name: "duration", Value: 60 * time.Second, Usage: "Test duration"},
			&cli.BoolFlag{Name: "old", Value: false, Usage: "Use old (slow) proof generation"},
		},
		Action: func(c *cli.Context) error {
			serverAddr := c.String("server-addr")
			rangeSize := c.Int("range-size")
			numClients := c.Int("num-clients")
			rpsPerClient := c.Int("rps-per-client")
			maxConcurrent := c.Int("max-concurrent")
			duration := c.Duration("duration")
			useOld := c.Bool("old")

			// Get server info.
			conn, err := dialGRPC(serverAddr)
			if err != nil {
				return err
			}
			info, err := proofpb.NewProofServiceClient(conn).GetInfo(context.Background(), &proofpb.GetInfoRequest{})
			conn.Close()
			if err != nil {
				return fmt.Errorf("GetInfo failed: %w", err)
			}
			log.Printf("Server info: latest_block=%d state_root=%s", info.LatestBlock, info.StateRoot)

			firstBlock := dataset.FIRST_BLOCK
			if info.LatestBlock-firstBlock+1 < uint64(rangeSize) {
				return fmt.Errorf("server has %d blocks, need at least %d for range-size=%d",
					info.LatestBlock-firstBlock+1, rangeSize, rangeSize)
			}

			selector, err := benchutil.NewWeightedAccountSelector(c.String("accounts-list"))
			if err != nil {
				return err
			}
			log.Printf("Loaded %d weighted accounts", selector.Size())

			offeredRPS := numClients * rpsPerClient
			log.Printf("Starting open-loop test: %d clients x %d rps = %d offered rps, duration %s, max_concurrent %d",
				numClients, rpsPerClient, offeredRPS, duration, maxConcurrent)

			// Atomic counters for aggregate stats.
			var sent, completed, dropped, clientErrors, serverErrors atomic.Int64

			ctx, cancel := context.WithTimeout(context.Background(), duration)
			defer cancel()

			var wg sync.WaitGroup
			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientID int) {
					defer wg.Done()

					clientConn, err := dialGRPC(serverAddr)
					if err != nil {
						log.Printf("client %d: dial failed: %v", clientID, err)
						return
					}
					defer clientConn.Close()
					cl := proofpb.NewProofServiceClient(clientConn)

					sem := make(chan struct{}, maxConcurrent)
					ticker := time.NewTicker(time.Second / time.Duration(rpsPerClient))
					defer ticker.Stop()

					var requestWg sync.WaitGroup
					defer requestWg.Wait()

					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							// Non-blocking semaphore acquire.
							select {
							case sem <- struct{}{}:
								// Acquired.
							default:
								dropped.Add(1)
								continue
							}

							sent.Add(1)
							account := selector.Pick()
							endBlock := info.LatestBlock
							startBlock := endBlock - uint64(rangeSize) + 1

							req := &proofpb.GetProofRequest{
								Account:    account,
								StartBlock: startBlock,
								EndBlock:   endBlock,
							}

							requestWg.Add(1)
							go func() {
								defer requestWg.Done()
								defer func() { <-sem }()

								_, reqErr := fetchProofStream(ctx, cl, req, useOld)
								if reqErr != nil {
									// Don't count cancellations from test ending.
									if ctx.Err() != nil {
										return
									}
									if isClientError(reqErr) {
										clientErrors.Add(1)
									} else {
										serverErrors.Add(1)
									}
									return
								}
								completed.Add(1)
							}()
						}
					}
				}(i)
			}

			wg.Wait()

			// Print summary.
			fmt.Println()
			fmt.Println("=== Open-Loop Results ===")
			fmt.Printf("Clients:       %d\n", numClients)
			fmt.Printf("Offered RPS:   %d\n", offeredRPS)
			fmt.Printf("Duration:      %s\n", duration)
			fmt.Printf("Sent:          %d\n", sent.Load())
			fmt.Printf("Completed:     %d\n", completed.Load())
			fmt.Printf("Dropped:       %d\n", dropped.Load())
			fmt.Printf("Client Errors: %d\n", clientErrors.Load())
			fmt.Printf("Server Errors: %d\n", serverErrors.Load())
			fmt.Println()

			return nil
		},
	}
}

// --- helpers ---

func dialGRPC(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)),
	)
}

func fetchProofStream(ctx context.Context, client proofpb.ProofServiceClient, req *proofpb.GetProofRequest, useOld bool) (*proofpb.GetProofResponse, error) {
	var stream grpc.ServerStreamingClient[proofpb.GetProofResponse]
	var err error
	if useOld {
		stream, err = client.GetOldProofStream(ctx, req)
	} else {
		stream, err = client.GetProofStream(ctx, req)
	}
	if err != nil {
		return nil, err
	}

	finalResp := &proofpb.GetProofResponse{}
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(chunk.RangeProofs) > 0 {
			finalResp.RangeProofs = append(finalResp.RangeProofs, chunk.RangeProofs...)
		}
		if chunk.ProofgenDurationNs > 0 {
			finalResp.ProofgenDurationNs = chunk.ProofgenDurationNs
		}
		if len(chunk.BalanceInfos) > 0 {
			finalResp.BalanceInfos = append(finalResp.BalanceInfos, chunk.BalanceInfos...)
		}
		if len(chunk.MptProofNodes) > 0 {
			finalResp.MptProofNodes = chunk.MptProofNodes
		}
		if len(chunk.CurrentBalance) > 0 {
			finalResp.CurrentBalance = chunk.CurrentBalance
		}
		if chunk.MptBlockNumber > 0 {
			finalResp.MptBlockNumber = chunk.MptBlockNumber
		}
	}
	return finalResp, nil
}

func verifyResponse(resp *proofpb.GetProofResponse, addr common.Address, precomputed *config.PrecomputedData, stateRoot common.Hash) error {
	rangeProofs := make([]*proof.RangeProof, len(resp.RangeProofs))
	for i, rp := range resp.RangeProofs {
		rangeProofs[i] = server.RangeProofFromProto(rp)
	}
	balanceInfos := make([]*tree.HistoricalBalance, len(resp.BalanceInfos))
	for i, bi := range resp.BalanceInfos {
		balanceInfos[i] = server.BalanceInfoFromProto(bi)
	}

	var startingVersion uint64
	var endingVersion uint64
	if len(balanceInfos) > 0 {
		startingVersion = math.MaxUint64
		for _, b := range balanceInfos {
			if b.Version < startingVersion {
				startingVersion = b.Version
			}
			if b.Version > endingVersion {
				endingVersion = b.Version
			}
		}
	}

	var currentBalance *tree.CurrentBalance
	if len(resp.CurrentBalance) > 0 {
		currentBalance = &tree.CurrentBalance{}
		if err := currentBalance.UnmarshalBinary(resp.CurrentBalance); err != nil {
			log.Printf("Failed to unmarshal current balance: %v", err)
			currentBalance = nil
		}
	}

	return proof.VerifyNewRangeProofs(addr, startingVersion, endingVersion, rangeProofs, balanceInfos, precomputed, resp.MptProofNodes, stateRoot, currentBalance)
}

func isClientError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.OutOfRange, codes.NotFound, codes.InvalidArgument:
		return true
	default:
		return false
	}
}

type samuraiQuerySummary struct {
	Account      string
	StartBlock   uint64
	EndBlock     uint64
	StartVersion uint64
	EndVersion   uint64
	ProofgenDur  time.Duration
	ResponseDur  time.Duration
	VerifyDur    time.Duration
	Verified     bool
	VerifyOK     bool
	PayloadBytes int64
	RangeProofs  int
	BalanceInfos int
	ServerErr    string
	ClientErr    string
}

func printSamuraiQuerySummary(w io.Writer, s samuraiQuerySummary) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "─── Samurai Query Summary ───────────────────────────")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-20s %s\n", "Account:", truncateAddr(s.Account))
	fmt.Fprintf(w, "  %-20s %d\n", "Starting Block:", s.StartBlock)
	fmt.Fprintf(w, "  %-20s %d\n", "Ending Block:", s.EndBlock)
	fmt.Fprintf(w, "  %-20s %d blocks\n", "Range:", s.EndBlock-s.StartBlock+1)
	if s.BalanceInfos > 0 {
		fmt.Fprintf(w, "  %-20s %d → %d (%d total)\n", "Versions:", s.StartVersion, s.EndVersion, s.EndVersion-s.StartVersion+1)
	}
	fmt.Fprintln(w)
	if s.ProofgenDur > 0 {
		fmt.Fprintf(w, "  %-20s %s\n", "Proofgen Latency:", s.ProofgenDur.Round(100*time.Microsecond))
	}
	fmt.Fprintf(w, "  %-20s %s\n", "Response Latency:", s.ResponseDur.Round(100*time.Microsecond))
	if s.Verified {
		fmt.Fprintf(w, "  %-20s %s\n", "Verify Latency:", s.VerifyDur.Round(100*time.Microsecond))
		if s.VerifyOK {
			fmt.Fprintf(w, "  %-20s ✓ PASSED\n", "Verify Result:")
		} else {
			fmt.Fprintf(w, "  %-20s ✗ FAILED\n", "Verify Result:")
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-20s %s\n", "Payload Size:", humanBytes(s.PayloadBytes))
	fmt.Fprintf(w, "  %-20s %d\n", "Range Proofs:", s.RangeProofs)
	fmt.Fprintf(w, "  %-20s %d\n", "Balance Infos:", s.BalanceInfos)
	if s.ServerErr != "" {
		fmt.Fprintf(w, "  %-20s %s\n", "Server Error:", s.ServerErr)
	}
	if s.ClientErr != "" {
		fmt.Fprintf(w, "  %-20s %s\n", "Client Error:", s.ClientErr)
	}
	if s.ServerErr == "" && s.ClientErr == "" {
		fmt.Fprintf(w, "  %-20s none\n", "Errors:")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "─────────────────────────────────────────────────────")
}

func truncateAddr(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}

func humanBytes(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
}

func setupPrecomputedData(paramsDir string) (*config.PrecomputedData, error) {
	log.Println("Setting up precomputed data...")
	start := time.Now()
	srs, err := kzg.SetupSRS(tree.SegmentTreeSize)
	if err != nil {
		return nil, fmt.Errorf("setup SRS: %w", err)
	}
	V, weights, weightCommits := kzg.LoadBarycentricData(tree.SegmentTreeSize, srs, paramsDir)
	log.Printf("Precomputed data setup took %v", time.Since(start))
	return &config.PrecomputedData{
		V:             V,
		Weights:       weights,
		WeightCommits: weightCommits,
		SRS:           srs,
	}, nil
}
