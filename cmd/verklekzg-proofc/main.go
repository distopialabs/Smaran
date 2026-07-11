package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	proofpb "github.com/nepal80m/samurai/api/proto/verkle/v1"
	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/verklekzg/proof"
	"github.com/nepal80m/samurai/internal/verklekzg/tree"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	app := &cli.App{
		Name:  "verklekzg-proofc",
		Usage: "Verkle-KZG gRPC proof client",
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
		Usage: "Query a single range proof",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "server-addr", Value: "localhost:50053", Usage: "gRPC server address"},
			&cli.StringFlag{Name: "account", Required: true, Usage: "Account address (0x hex)"},
			&cli.Uint64Flag{Name: "start-block", Required: true, Usage: "Start block number"},
			&cli.Uint64Flag{Name: "end-block", Required: true, Usage: "End block number"},
			&cli.BoolFlag{Name: "verify", Value: true, Usage: "Verify each block proof locally"},
			&cli.StringFlag{Name: "params-dir", Value: "data/params/verklekzg", Usage: "Directory for precomputed SRS/barycentric files"},
		},
		Action: func(c *cli.Context) error {
			conn, err := grpc.NewClient(c.String("server-addr"),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return fmt.Errorf("dial: %w", err)
			}
			defer conn.Close()
			client := proofpb.NewVerkleProofServiceClient(conn)

			account := c.String("account")
			startBlock := c.Uint64("start-block")
			endBlock := c.Uint64("end-block")

			req := &proofpb.GetRangeProofRequest{
				Account:    account,
				StartBlock: startBlock,
				EndBlock:   endBlock,
			}

			respStart := time.Now()
			proofs, proofgenNs, fetchErr := callRangeProof(context.Background(), client, req)
			respDur := time.Since(respStart)

			if fetchErr != nil {
				s := querySummary{
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
				printQuerySummary(os.Stderr, s)
				return fmt.Errorf("GetRangeProof: %w", fetchErr)
			}

			var payloadBytes int64
			for _, bp := range proofs {
				payloadBytes += int64(len(bp.VerkleProof))
			}

			doVerify := c.Bool("verify")
			var verifyDur time.Duration
			var verifiedCount, failedCount int
			if doVerify {
				treeCfg, cfgErr := tree.NewTreeConfig(c.String("params-dir"))
				if cfgErr != nil {
					return fmt.Errorf("init tree config for verification: %w", cfgErr)
				}
				verifyStart := time.Now()
				for _, bp := range proofs {
					if err := verifyBlockProof(bp, treeCfg); err != nil {
						failedCount++
					} else {
						verifiedCount++
					}
				}
				verifyDur = time.Since(verifyStart)
			}

			printQuerySummary(os.Stderr, querySummary{
				Account:       account,
				StartBlock:    startBlock,
				EndBlock:      endBlock,
				ProofgenDur:   time.Duration(proofgenNs),
				ResponseDur:        respDur,
				VerifyDur:     verifyDur,
				Verified:      doVerify,
				VerifiedCount: verifiedCount,
				FailedCount:   failedCount,
				PayloadBytes:  payloadBytes,
				BlockProofs:   len(proofs),
			})

			return nil
		},
	}
}

// --- bench subcommand ---

func benchCmd() *cli.Command {
	return &cli.Command{
		Name:  "bench",
		Usage: "Run a proof generation benchmark",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "server-addr", Value: "localhost:50053", Usage: "gRPC server address"},
			&cli.IntFlag{Name: "range-size", Value: 50000, Usage: "Block range size per query"},
			&cli.IntFlag{Name: "num-clients", Value: 1, Usage: "Number of concurrent client goroutines"},
			&cli.StringFlag{Name: "accounts-list", Required: true, Usage: "CSV with accounts sorted by update count desc"},
			&cli.DurationFlag{Name: "duration", Value: 60 * time.Second, Usage: "Benchmark duration"},
			&cli.BoolFlag{Name: "verify", Value: true, Usage: "Verify proofs locally"},
			&cli.StringFlag{Name: "output-dir", Value: benchutil.DefaultOutputDir, Usage: "Root directory for benchmark output"},
			&cli.StringFlag{Name: "params-dir", Value: "data/params/verklekzg", Usage: "Directory for precomputed SRS/barycentric files"},
		},
		Action: func(c *cli.Context) error {
			conn, err := grpc.NewClient(c.String("server-addr"),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return fmt.Errorf("dial: %w", err)
			}
			defer conn.Close()
			client := proofpb.NewVerkleProofServiceClient(conn)

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

			doVerify := c.Bool("verify")
			var treeCfg *tree.TreeConfig
			if doVerify {
				treeCfg, err = tree.NewTreeConfig(c.String("params-dir"))
				if err != nil {
					return fmt.Errorf("init tree config for verification: %w", err)
				}
			}

			cfg := benchutil.ProofBenchConfig{
				ServerAddr: c.String("server-addr"),
				RangeSize:  rangeSize,
				NumClients: c.Int("num-clients"),
				Duration:   c.Duration("duration"),
				FirstBlock: firstBlock,
			}

			allStats := make([]benchutil.ClientStats, cfg.NumClients)
			var wg sync.WaitGroup
			deadline := time.Now().Add(cfg.Duration)
			benchStart := time.Now()

			for i := 0; i < cfg.NumClients; i++ {
				wg.Add(1)
				go func(clientID int) {
					defer wg.Done()

					clientConn, err := grpc.NewClient(cfg.ServerAddr,
						grpc.WithTransportCredentials(insecure.NewCredentials()),
						grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
					)
					if err != nil {
						log.Printf("client %d: dial failed: %v", clientID, err)
						return
					}
					defer clientConn.Close()
					cl := proofpb.NewVerkleProofServiceClient(clientConn)

					var stats benchutil.ClientStats

					for time.Now().Before(deadline) {
						account := selector.Pick()
						endBlock := info.LatestBlock
						startBlock := endBlock - uint64(rangeSize) + 1

						req := &proofpb.GetRangeProofRequest{
							Account:    account,
							StartBlock: startBlock,
							EndBlock:   endBlock,
						}

						respStart := time.Now()
						proofs, proofgenNs, reqErr := callRangeProof(context.Background(), cl, req)
						respNs := time.Since(respStart).Nanoseconds()

						if reqErr != nil {
							if isClientError(reqErr) {
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
						stats.TotalProofgenNs += proofgenNs

						for _, bp := range proofs {
							stats.TotalPayloadBytes += int64(len(bp.VerkleProof))
						}

						if doVerify && treeCfg != nil {
							vStart := time.Now()
							for _, bp := range proofs {
								if err := verifyBlockProof(bp, treeCfg); err != nil {
									stats.TotalVerifyFailures++
									break
								}
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

			if err := benchutil.WriteSummaryFile(c.String("output-dir"), "verklekzg", cfg, agg, wallDuration); err != nil {
				log.Printf("warning: failed to write summary file: %v", err)
			}

			return nil
		},
	}
}

// --- helpers ---

func callRangeProof(ctx context.Context, client proofpb.VerkleProofServiceClient, req *proofpb.GetRangeProofRequest) ([]*proofpb.BlockProof, int64, error) {
	stream, err := client.GetRangeProof(ctx, req)
	if err != nil {
		return nil, 0, err
	}

	var proofs []*proofpb.BlockProof
	for {
		bp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return proofs, 0, err
		}
		proofs = append(proofs, bp)
	}

	var proofgenNs int64
	if trailer := stream.Trailer(); trailer != nil {
		if vals := trailer.Get("proofgen_duration_ns"); len(vals) > 0 {
			proofgenNs, _ = strconv.ParseInt(vals[0], 10, 64)
		}
	}

	return proofs, proofgenNs, nil
}

func verifyBlockProof(bp *proofpb.BlockProof, cfg *tree.TreeConfig) error {
	var vkProof proof.VerkleKZGProof
	if err := json.Unmarshal(bp.VerkleProof, &vkProof); err != nil {
		return fmt.Errorf("unmarshal proof: %w", err)
	}
	return proof.VerifyProof(bp.RootCommitment, &vkProof, cfg)
}

type querySummary struct {
	Account       string
	StartBlock    uint64
	EndBlock      uint64
	ProofgenDur   time.Duration
	ResponseDur   time.Duration
	VerifyDur     time.Duration
	Verified      bool
	VerifiedCount int
	FailedCount   int
	PayloadBytes  int64
	BlockProofs   int
	ServerErr     string
	ClientErr     string
}

func printQuerySummary(w io.Writer, s querySummary) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "─── Verkle-KZG Query Summary ────────────────────────")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-20s %s\n", "Account:", truncateAddr(s.Account))
	fmt.Fprintf(w, "  %-20s %d\n", "Starting Block:", s.StartBlock)
	fmt.Fprintf(w, "  %-20s %d\n", "Ending Block:", s.EndBlock)
	fmt.Fprintf(w, "  %-20s %d blocks\n", "Range:", s.EndBlock-s.StartBlock+1)
	fmt.Fprintf(w, "  %-20s %d\n", "Block Proofs:", s.BlockProofs)
	fmt.Fprintln(w)
	if s.ProofgenDur > 0 {
		fmt.Fprintf(w, "  %-20s %s\n", "Proofgen Latency:", s.ProofgenDur.Round(100*time.Microsecond))
	}
	fmt.Fprintf(w, "  %-20s %s\n", "Response Latency:", s.ResponseDur.Round(100*time.Microsecond))
	if s.Verified {
		fmt.Fprintf(w, "  %-20s %s\n", "Verify Latency:", s.VerifyDur.Round(100*time.Microsecond))
		total := s.VerifiedCount + s.FailedCount
		if s.FailedCount == 0 {
			fmt.Fprintf(w, "  %-20s PASSED (%d/%d blocks)\n", "Verify Result:", s.VerifiedCount, total)
		} else {
			fmt.Fprintf(w, "  %-20s FAILED (%d/%d blocks failed)\n", "Verify Result:", s.FailedCount, total)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-20s %s\n", "Payload Size:", humanBytes(s.PayloadBytes))
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

func dialGRPC(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
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
		},
		Action: func(c *cli.Context) error {
			serverAddr := c.String("server-addr")
			rangeSize := c.Int("range-size")
			numClients := c.Int("num-clients")
			rpsPerClient := c.Int("rps-per-client")
			maxConcurrent := c.Int("max-concurrent")
			duration := c.Duration("duration")

			conn, err := dialGRPC(serverAddr)
			if err != nil {
				return err
			}
			info, err := proofpb.NewVerkleProofServiceClient(conn).GetInfo(context.Background(), &proofpb.GetInfoRequest{})
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
					cl := proofpb.NewVerkleProofServiceClient(clientConn)

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
							select {
							case sem <- struct{}{}:
							default:
								dropped.Add(1)
								continue
							}

							sent.Add(1)
							account := selector.Pick()
							endBlock := info.LatestBlock
							startBlock := endBlock - uint64(rangeSize) + 1

							req := &proofpb.GetRangeProofRequest{
								Account:    account,
								StartBlock: startBlock,
								EndBlock:   endBlock,
							}

							requestWg.Add(1)
							go func() {
								defer requestWg.Done()
								defer func() { <-sem }()

								_, _, reqErr := callRangeProof(ctx, cl, req)
								if reqErr != nil {
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
