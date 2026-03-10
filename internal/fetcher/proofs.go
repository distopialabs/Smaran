package fetcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/logging"
)

var log = logging.GetLogger("fetcher")

type jsonRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

// RunProofFetcher fetches eth_getProof results for an address across a block range and writes them to disk.
func RunProofFetcher(alchemyURL string, address common.Address, startBlock, endBlock uint64, outDir string, rps int) error {
	if startBlock > endBlock {
		return fmt.Errorf("start block %d greater than end block %d", startBlock, endBlock)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	if rps <= 0 {
		rps = 1
	}
	interval := time.Second / time.Duration(rps)
	throttle := time.NewTicker(interval)
	defer throttle.Stop()

	addrHex := address.Hex()
	idCounter := 1

	for bn := startBlock; bn <= endBlock; bn++ {
		filename := fmt.Sprintf("%s_%d.json", addrHex, bn)
		filepathOut := filepath.Join(outDir, filename)

		if fi, err := os.Stat(filepathOut); err == nil && fi.Size() > 0 {
			continue
		}

		<-throttle.C

		reqBody := jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      idCounter,
			Method:  "eth_getProof",
			Params:  []interface{}{addrHex, []string{}, fmt.Sprintf("0x%x", bn)},
		}
		idCounter++
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}

		var lastErr error
		for attempt := 0; attempt < 8; attempt++ {
			req, err := http.NewRequest("POST", alchemyURL, bytes.NewReader(bodyBytes))
			if err != nil {
				return fmt.Errorf("new request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "samurai-proof-fetcher/1.0")

			resp, err := httpClient.Do(req)
			if err != nil {
				lastErr = err
			} else {
				func() {
					defer resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						respBytes, err := io.ReadAll(resp.Body)
						if err != nil {
							lastErr = err
							return
						}
						var decoded jsonRPCResponse
						if err := json.Unmarshal(respBytes, &decoded); err != nil {
							lastErr = fmt.Errorf("decode response: %w", err)
							return
						}
						if decoded.Error != nil {
							lastErr = fmt.Errorf("rpc error code %d: %s", decoded.Error.Code, decoded.Error.Message)
						} else if len(decoded.Result) == 0 || string(decoded.Result) == "null" {
							lastErr = fmt.Errorf("empty result for block %d", bn)
						} else {
							wrapped := map[string]any{
								"address":     addrHex,
								"blockNumber": bn,
								"result":      json.RawMessage(decoded.Result),
							}
							fileBytes, err := json.MarshalIndent(wrapped, "", "  ")
							if err != nil {
								lastErr = fmt.Errorf("encode file: %w", err)
								return
							}
							if err := os.WriteFile(filepathOut, fileBytes, 0o644); err != nil {
								lastErr = fmt.Errorf("write file: %w", err)
								return
							}
							lastErr = nil
						}
					} else {
						respBody, _ := io.ReadAll(resp.Body)
						lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
					}
				}()

				if lastErr == nil {
					break
				}
			}

			backoff := time.Duration(250*(1<<attempt)) * time.Millisecond
			if backoff > 10*time.Second {
				backoff = 10 * time.Second
			}
			time.Sleep(backoff + time.Duration(attempt*37%251)*time.Millisecond)
		}

		if lastErr != nil {
			log.Warningf("failed to fetch proof for block %d: %v", bn, lastErr)
		}

		if bn%1000 == 0 {
			log.Infof("progress: fetched up to block %d", bn)
		}
	}

	log.Infof("completed fetching proofs for %s from %d to %d into %s", addrHex, startBlock, endBlock, outDir)
	return nil
}
