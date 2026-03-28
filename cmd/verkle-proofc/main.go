// Package main provides a CLI client for the baseline-verkle gRPC proof service.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	verkle "github.com/ethereum/go-verkle"
	proofpb "github.com/nepal80m/samurai/api/proto/verkle/v1"
	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/verkle/keys"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	app := &cli.App{
		Name:  "verkle-proofc",
		Usage: "Verkle gRPC proof client",
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
		Usage: "Query a single range proof",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "server-addr", Value: "localhost:50053", Usage: "gRPC server address"},
			&cli.StringFlag{Name: "account", Required: true, Usage: "Account address (0x hex)"},
			&cli.Uint64Flag{Name: "start-block", Required: true, Usage: "Start block number"},
			&cli.Uint64Flag{Name: "end-block", Required: true, Usage: "End block number"},
			&cli.BoolFlag{Name: "verify", Value: true, Usage: "Verify each block proof locally"},
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
				s := verkleQuerySummary{
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
				printVerkleQuerySummary(os.Stderr, s)
				return fmt.Errorf("GetRangeProof: %w", fetchErr)
			}

			// Calculate payload size.
			var payloadBytes int64
			for _, bp := range proofs {
				payloadBytes += int64(len(bp.VerkleProof) + len(bp.StateDiff))
			}

			// Verify if requested.
			doVerify := c.Bool("verify")
			var verifyDur time.Duration
			var verifiedCount, failedCount int
			if doVerify {
				verifyStart := time.Now()
				for _, bp := range proofs {
					_, verErr := verifyBlockProof(bp, account)
					if verErr != nil {
						failedCount++
					} else {
						verifiedCount++
					}
				}
				verifyDur = time.Since(verifyStart)
			}

			printVerkleQuerySummary(os.Stderr, verkleQuerySummary{
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

			// Call GetInfo.
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
							}
							continue
						}

						stats.TotalRequests++
						stats.TotalResponseNs += respNs
						stats.TotalProofgenNs += proofgenNs

						// Payload size: sum of proof fields.
						for _, bp := range proofs {
							stats.TotalPayloadBytes += int64(len(bp.VerkleProof) + len(bp.StateDiff))
						}

						if doVerify {
							vStart := time.Now()
							for _, bp := range proofs {
								if _, err := verifyBlockProof(bp, account); err != nil {
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

			if err := benchutil.WriteSummaryFile(c.String("output-dir"), "verkle", cfg, agg, wallDuration); err != nil {
				log.Printf("warning: failed to write summary file: %v", err)
			}

			return nil
		},
	}
}

// --- helpers ---

// callRangeProof opens a streaming RPC, collects all BlockProofs, and reads
// proofgen_duration_ns from trailing metadata.
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

func verifyBlockProof(bp *proofpb.BlockProof, account string) (bool, error) {
	var vp verkle.VerkleProof
	if err := json.Unmarshal(bp.VerkleProof, &vp); err != nil {
		return false, fmt.Errorf("unmarshal VerkleProof: %w", err)
	}
	var sd verkle.StateDiff
	if err := json.Unmarshal(bp.StateDiff, &sd); err != nil {
		return false, fmt.Errorf("unmarshal StateDiff: %w", err)
	}

	if err := verkle.Verify(&vp, bp.RootCommitment, bp.RootCommitment, sd); err != nil {
		return false, err
	}

	// Verify the account key is present in the state diff.
	addrStr := strings.TrimPrefix(account, "0x")
	addrBytes, err := hexToAddr(addrStr)
	if err != nil {
		return false, err
	}
	treeKey := keys.GetTreeKeyForBasicData(addrBytes)
	stem := treeKey[:31]
	suffix := treeKey[31]

	for _, ssd := range sd {
		if bytes.Equal(ssd.Stem[:], stem) {
			for _, suffDiff := range ssd.SuffixDiffs {
				if suffDiff.Suffix == suffix {
					return true, nil
				}
			}
		}
	}

	return true, nil
}

func hexToAddr(hexStr string) ([20]byte, error) {
	var addr [20]byte
	if len(hexStr) != 40 {
		return addr, fmt.Errorf("invalid address length: %s", hexStr)
	}
	for i := 0; i < 20; i++ {
		hi := unhex(hexStr[2*i])
		lo := unhex(hexStr[2*i+1])
		if hi == 0xFF || lo == 0xFF {
			return addr, fmt.Errorf("invalid hex character in address")
		}
		addr[i] = hi<<4 | lo
	}
	return addr, nil
}

func unhex(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	default:
		return 0xFF
	}
}

type verkleQuerySummary struct {
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

func printVerkleQuerySummary(w io.Writer, s verkleQuerySummary) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "─── Verkle Query Summary ────────────────────────────")
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
			fmt.Fprintf(w, "  %-20s ✓ PASSED (%d/%d blocks)\n", "Verify Result:", s.VerifiedCount, total)
		} else {
			fmt.Fprintf(w, "  %-20s ✗ FAILED (%d/%d blocks failed)\n", "Verify Result:", s.FailedCount, total)
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
