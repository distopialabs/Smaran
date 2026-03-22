// Package main provides a CLI client for the Samurai gRPC proof service.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"
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
			&cli.StringFlag{Name: "account", Required: true, Usage: "Account address (0x hex)"},
			&cli.Uint64Flag{Name: "start-block", Required: true, Usage: "Start block number"},
			&cli.Uint64Flag{Name: "end-block", Required: true, Usage: "End block number"},
			&cli.StringFlag{Name: "params-dir", Value: "./data/params", Usage: "Path to crypto params"},
			&cli.StringFlag{Name: "state-root", Value: "", Usage: "MPT state root hash (hex) for verification"},
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

			req := &proofpb.GetProofRequest{
				Account:    c.String("account"),
				StartBlock: c.Uint64("start-block"),
				EndBlock:   c.Uint64("end-block"),
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			resp, err := fetchProofStream(ctx, client, req)
			if err != nil {
				return fmt.Errorf("GetProofStream failed: %w", err)
			}

			addr := common.HexToAddress(c.String("account"))
			return verifyResponse(resp, addr, precomputed, stateRoot)
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
			&cli.BoolFlag{Name: "verify", Value: false, Usage: "Verify proofs (requires --params-dir)"},
			&cli.StringFlag{Name: "params-dir", Value: "./data/params", Usage: "Path to crypto params (for verification)"},
			&cli.StringFlag{Name: "state-root", Value: "", Usage: "MPT state root hash (hex) for verification"},
		},
		Action: func(c *cli.Context) error {
			conn, err := dialGRPC(c.String("server-addr"))
			if err != nil {
				return err
			}
			defer conn.Close()
			client := proofpb.NewProofServiceClient(conn)

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

					rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(clientID)))
					var stats benchutil.ClientStats

					for time.Now().Before(deadline) {
						account := selector.Pick()
						startBlock := benchutil.RandomStartBlock(rng, firstBlock, info.LatestBlock, rangeSize)
						endBlock := startBlock + uint64(rangeSize) - 1

						req := &proofpb.GetProofRequest{
							Account:    account,
							StartBlock: startBlock,
							EndBlock:   endBlock,
						}

						e2eStart := time.Now()
						resp, reqErr := fetchProofStream(context.Background(), cl, req)
						e2eNs := time.Since(e2eStart).Nanoseconds()

						if reqErr != nil {
							if isClientError(reqErr) {
								stats.TotalClientErrors++
							} else {
								stats.TotalServerErrors++
							}
							continue
						}

						stats.TotalRequests++
						stats.TotalE2ENs += e2eNs
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

			if err := benchutil.WriteSummaryFile("samurai", cfg, agg, wallDuration); err != nil {
				log.Printf("warning: failed to write summary file: %v", err)
			}

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

func fetchProofStream(ctx context.Context, client proofpb.ProofServiceClient, req *proofpb.GetProofRequest) (*proofpb.GetProofResponse, error) {
	stream, err := client.GetProofStream(ctx, req)
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

	var startingVersion uint64 = math.MaxUint64
	var endingVersion uint64 = 0
	for _, b := range balanceInfos {
		if b.Version < startingVersion {
			startingVersion = b.Version
		}
		if b.Version > endingVersion {
			endingVersion = b.Version
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
