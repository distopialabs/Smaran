// Binary ktbench benchmarks a Key Transparency HTTP/JSON server.
//
// It opens persistent TCP connections and uses HTTP pipelining to
// saturate the server: requests are written back-to-back without
// waiting for responses, which are drained asynchronously by a
// separate reader goroutine per connection.
//
// See KT.md § "Benchmarking Client" for the full specification.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/hash"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/logging"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/tree"
)

var log = logging.GetLogger("ktbench")

type putRequest struct {
	User []byte `json:"user"`
	Key  []byte `json:"key"`
}

type getRequest struct {
	User       []byte `json:"user"`
	UseCaching bool   `json:"use_caching"`
}

type optiksQueryResult struct {
	Value                         []byte     `json:"value"`
	CurrentVersion                uint64     `json:"current_version"`
	NextVersionNonMembershipProof [][]byte   `json:"next_version_non_membership_proof"`
	VersionProofs                 [][][]byte `json:"version_proofs"`
	CurrentVersionEpoch           uint64     `json:"current_version_epoch"`
	OldVersions                   [][]byte   `json:"old_versions"`
	OldVersionEpochs              []uint64   `json:"old_version_epochs"`
	CommonProofPrefix             [][]byte   `json:"common_proof_prefix"`
}

type samuraiRangeProofJSON struct {
	Idx                  int               `json:"idx"`
	Layer                int               `json:"layer"`
	Commitment           []byte            `json:"commitment"`
	Proof                []byte            `json:"proof"`
	BlockRange           *proof.BlockRange `json:"block_range"`
	DependentCommitments []int             `json:"dependent_commitments"`
}

type samuraiQueryResult struct {
	Value              []byte                  `json:"value"`
	CurrentVersion     uint64                  `json:"current_version"`
	MptProof           [][]byte                `json:"mpt_proof"`
	CommitmentHash     []byte                  `json:"commitment_hash"`
	SamuraiProofs      []samuraiRangeProofJSON `json:"samurai_proofs"`
	HistoricalBalances [][]byte                `json:"historical_balances"`
}

type getCommitmentResponse struct {
	Commitment []byte `json:"commitment"`
}

type runClientMetrics struct {
	TotalRequestsCompleted    int64
	TotalProofGenLatency      time.Duration
	TotalVerifyLatency        time.Duration
	TotalLatency              time.Duration
	TotalPayloadSize          int64
	TotalCommonPrefixSize     int64
	TotalProofElements        int64
	TotalCommonPrefixElements int64
}

func main() {
	addr := flag.String("addr", "127.0.0.1:3191", "KT server address")
	protocol := flag.String("protocol", "samurai", "protocol: 'samurai' or 'optiks'")
	numUsers := flag.Int("num-users", 1000, "number of users to simulate")
	numLoadClients := flag.Int("num-load-clients", 1, "concurrent clients during load phase")
	numRunClients := flag.Int("num-run-clients", 1, "concurrent clients during run phase")
	numVersions := flag.Int("num-versions", 5, "key updates per user")
	useCaching := flag.Bool("use_caching", false, "pass use_caching=true in Get requests")
	runDurationSecs := flag.Int("run-duration-secs", 30, "how long the run phase lasts (seconds)")
	runMode := flag.String("run-mode", "get", "run phase mode: 'get' or 'put'")
	paramsDir := flag.String("params-dir", "./data/params", "directory for precomputed cryptographic parameters (samurai only)")
	flag.Parse()

	if *runMode != "get" && *runMode != "put" {
		log.Fatalf("invalid -run-mode %q: must be 'get' or 'put'", *runMode)
	}

	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		if err := logging.SetLevel(lvl); err != nil {
			log.Fatalf("invalid LOG_LEVEL %q: %v", lvl, err)
		}
	}

	var precomputedData *config.PrecomputedData
	if *protocol == "samurai" {
		srs, err := kzg.SetupSRS(tree.SegmentTreeSize)
		if err != nil {
			log.Fatalf("failed to setup SRS: %v", err)
		}
		V, weights, weightCommits := kzg.LoadBarycentricData(tree.SegmentTreeSize, srs, *paramsDir)
		precomputedData = &config.PrecomputedData{
			V:             V,
			Weights:       weights,
			WeightCommits: weightCommits,
			SRS:           srs,
		}
	}

	log.Infof("=== KT Benchmark Client ===")
	log.Infof("Server: %s, Protocol: %s", *addr, *protocol)
	log.Infof("Users: %d, Versions/user: %d, Load clients: %d, Run clients: %d, Run mode: %s",
		*numUsers, *numVersions, *numLoadClients, *numRunClients, *runMode)

	// Load Phase: distribute users [0, numUsers-1] among load-client goroutines.
	// See KT.md § "Benchmarking Client" — IDs are both inclusive.
	log.Infof("--- Load Phase ---")
	loadStart := time.Now()

	var wg sync.WaitGroup
	usersPerClient := *numUsers / *numLoadClients
	remainder := *numUsers % *numLoadClients

	offset := 0
	for i := 0; i < *numLoadClients; i++ {
		count := usersPerClient
		if i < remainder {
			count++
		}
		userIDStart := offset
		userIDEnd := offset + count - 1
		offset += count

		wg.Add(1)
		go func(clientID, idStart, idEnd, versions int) {
			defer wg.Done()
			runLoadClient(clientID, *addr, idStart, idEnd, versions)
		}(i, userIDStart, userIDEnd, *numVersions)
	}

	wg.Wait()
	log.Infof("Load phase complete in %v. Sleeping 2 seconds...", time.Since(loadStart))
	time.Sleep(2 * time.Second)

	// Run Phase: each goroutine sends synchronous requests for run-duration-secs.
	// See KT.md § "Run Phase".
	log.Infof("--- Run Phase (%d clients, %ds, mode=%s) ---", *numRunClients, *runDurationSecs, *runMode)
	runStart := time.Now()
	runDuration := time.Duration(*runDurationSecs) * time.Second
	log.Infof("RUN_PHASE_START_UNIX_NANO=%d", runStart.UnixNano())
	log.Infof("RUN_PHASE_START_RFC3339=%s", runStart.Format(time.RFC3339Nano))

	metricsCh := make(chan runClientMetrics, *numRunClients)
	var runWg sync.WaitGroup
	for i := 0; i < *numRunClients; i++ {
		runWg.Add(1)
		go func(clientID int) {
			defer runWg.Done()
			if *runMode == "put" {
				metricsCh <- runPutClient(clientID, *addr, *numUsers, runDuration)
			} else {
				metricsCh <- runRunClient(clientID, *addr, *numUsers, *useCaching, runDuration, *protocol, precomputedData)
			}
		}(i)
	}
	runWg.Wait()
	close(metricsCh)

	var total runClientMetrics
	for m := range metricsCh {
		total.TotalRequestsCompleted += m.TotalRequestsCompleted
		total.TotalProofGenLatency += m.TotalProofGenLatency
		total.TotalVerifyLatency += m.TotalVerifyLatency
		total.TotalLatency += m.TotalLatency
		total.TotalPayloadSize += m.TotalPayloadSize
		total.TotalCommonPrefixSize += m.TotalCommonPrefixSize
		total.TotalProofElements += m.TotalProofElements
		total.TotalCommonPrefixElements += m.TotalCommonPrefixElements
	}

	runEnd := time.Now()
	log.Infof("RUN_PHASE_END_UNIX_NANO=%d", runEnd.UnixNano())
	log.Infof("RUN_PHASE_END_RFC3339=%s", runEnd.Format(time.RFC3339Nano))
	log.Infof("Run phase complete in %v", runEnd.Sub(runStart))
	log.Infof("Total requests completed: %d", total.TotalRequestsCompleted)
	if *runMode == "get" {
		log.Infof("Total proof-gen latency: %v", total.TotalProofGenLatency)
		log.Infof("Total verify latency: %v", total.TotalVerifyLatency)
		log.Infof("Total latency: %v", total.TotalLatency)
		log.Infof("Total payload: %d bytes", total.TotalPayloadSize)
		log.Infof("Total common prefix size: %d bytes", total.TotalCommonPrefixSize)
		log.Infof("Total proof elements: %d", total.TotalProofElements)
		log.Infof("Total common prefix elements: %d", total.TotalCommonPrefixElements)
		if total.TotalRequestsCompleted > 0 {
			n := float64(total.TotalRequestsCompleted)
			log.Infof("Avg proof-gen latency: %s", time.Duration(float64(total.TotalProofGenLatency)/n))
			log.Infof("Avg verify latency: %s", time.Duration(float64(total.TotalVerifyLatency)/n))
			log.Infof("Avg latency: %s", time.Duration(float64(total.TotalLatency)/n))
			log.Infof("Avg payload: %.3f bytes", float64(total.TotalPayloadSize)/n)
			log.Infof("Avg common prefix size: %.3f bytes", float64(total.TotalCommonPrefixSize)/n)
			log.Infof("Avg proof elements: %.3f", float64(total.TotalProofElements)/n)
			log.Infof("Avg common prefix elements: %.3f", float64(total.TotalCommonPrefixElements)/n)
		}
	}
	log.Infof("Benchmark complete.")
}

// runLoadClient opens a single persistent TCP connection to the KT server and
// sends Put requests for every user in [userIDStart, userIDEnd] (inclusive),
// each receiving exactly numVersions updates. Requests are pipelined: the
// writer sends them back-to-back while a reader goroutine drains responses
// asynchronously.
func runLoadClient(clientID int, addr string, userIDStart, userIDEnd, numVersions int) {
	numUsers := userIDEnd - userIDStart + 1
	totalRequests := numUsers * numVersions

	log.Infof("[Client %d] users [%d, %d] (%d users), %d versions => %d requests",
		clientID, userIDStart, userIDEnd, numUsers, numVersions, totalRequests)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Errorf("[Client %d] connect failed: %v", clientID, err)
		return
	}
	defer conn.Close()

	// Reader goroutine: drains HTTP responses as they arrive.
	responsesDone := make(chan int, 1)
	br := bufio.NewReaderSize(conn, 64*1024)
	go func() {
		count := 0
		for count < totalRequests {
			resp, err := http.ReadResponse(br, nil)
			if err != nil {
				log.Errorf("[Client %d] read response %d: %v", clientID, count, err)
				break
			}
			// log.Debugf("[Client %d] received response %d: %s", clientID, count, resp.Status)
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			count++
		}
		responsesDone <- count
	}()

	// Writer: pipeline Put requests without waiting for responses.
	bw := bufio.NewWriterSize(conn, 256*1024)
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(clientID)))

	// Maintain a pending slice of user IDs that still need more versions.
	// Uniform random sampling via swap-remove when a user reaches numVersions.
	pending := make([]int, numUsers)
	for i := range pending {
		pending[i] = userIDStart + i
	}
	versionCount := make(map[int]int, numUsers)
	sentCount := 0

	for len(pending) > 0 {
		idx := rng.Intn(len(pending))
		userID := pending[idx]

		user := []byte(fmt.Sprintf("user:%d", userID))
		key := make([]byte, 64)
		rng.Read(key)

		body, _ := json.Marshal(putRequest{User: user, Key: key})

		fmt.Fprintf(bw, "POST /put HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: keep-alive\r\n\r\n", addr, len(body))
		bw.Write(body)

		sentCount++
		versionCount[userID]++
		if versionCount[userID] == numVersions {
			pending[idx] = pending[len(pending)-1]
			pending = pending[:len(pending)-1]
		}
	}

	if err := bw.Flush(); err != nil {
		log.Errorf("[Client %d] flush: %v", clientID, err)
	}

	log.Infof("[Client %d] sent %d requests, waiting for responses...", clientID, sentCount)

	received := <-responsesDone
	log.Infof("[Client %d] done: %d/%d responses received", clientID, received, sentCount)
}
// runPutClient opens a persistent TCP connection and sends synchronous Put
// requests for random users until the duration elapses. It returns only the
// total completed request count; latency breakdown is intentionally not tracked.
func runPutClient(clientID int, addr string, numUsers int, duration time.Duration) runClientMetrics {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Errorf("[Run %d] connect failed: %v", clientID, err)
		return runClientMetrics{}
	}
	defer conn.Close()

	bw := bufio.NewWriterSize(conn, 64*1024)
	br := bufio.NewReaderSize(conn, 256*1024)
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(clientID)*1000))

	var metrics runClientMetrics
	deadline := time.Now().Add(duration)

	for time.Now().Before(deadline) {
		userID := rng.Intn(numUsers)
		user := []byte(fmt.Sprintf("user:%d", userID))
		key := make([]byte, 64)
		rng.Read(key)

		body, _ := json.Marshal(putRequest{User: user, Key: key})

		fmt.Fprintf(bw, "POST /put HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: keep-alive\r\n\r\n", addr, len(body))
		bw.Write(body)
		if err := bw.Flush(); err != nil {
			log.Errorf("[Run %d] write: %v", clientID, err)
			break
		}
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			log.Errorf("[Run %d] read response: %v", clientID, err)
			break
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		metrics.TotalRequestsCompleted++
	}
	return metrics
}

// runRunClient opens a persistent TCP connection and sends synchronous Get
// requests for random users until the duration elapses. It returns aggregated
// metrics to the caller. Avg latency / payload are logged roughly every second.
// For the optiks protocol, each response is verified against the root commitment
// obtained once at startup. See KT.md § "Verification".
func runRunClient(clientID int, addr string, numUsers int, useCaching bool, duration time.Duration, protocol string, precomputedData *config.PrecomputedData) runClientMetrics {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Errorf("[Run %d] connect failed: %v", clientID, err)
		return runClientMetrics{}
	}
	defer conn.Close()

	bw := bufio.NewWriterSize(conn, 64*1024)
	br := bufio.NewReaderSize(conn, 256*1024)
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(clientID)*1000))

	// Each run-phase goroutine calls GetCommitment() once at startup.
	var rootHash common.Hash
	{
		commitBody := []byte("{}")
		fmt.Fprintf(bw, "POST /get_commitment HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: keep-alive\r\n\r\n", addr, len(commitBody))
		bw.Write(commitBody)
		if err := bw.Flush(); err != nil {
			log.Errorf("[Run %d] GetCommitment write: %v", clientID, err)
			return runClientMetrics{}
		}
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			log.Errorf("[Run %d] GetCommitment read: %v", clientID, err)
			return runClientMetrics{}
		}
		var commitResp getCommitmentResponse
		json.NewDecoder(resp.Body).Decode(&commitResp)
		resp.Body.Close()
		rootHash = common.BytesToHash(commitResp.Commitment)
		log.Infof("[Run %d] root commitment: %x", clientID, rootHash)
	}

	var metrics runClientMetrics
	deadline := time.Now().Add(duration)
	lastLogTime := time.Now()

	for time.Now().Before(deadline) {
		userID := rng.Intn(numUsers)
		user := []byte(fmt.Sprintf("user:%d", userID))
		body, _ := json.Marshal(getRequest{User: user, UseCaching: useCaching})

		reqStart := time.Now()

		fmt.Fprintf(bw, "POST /get HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: keep-alive\r\n\r\n", addr, len(body))
		bw.Write(body)
		if err := bw.Flush(); err != nil {
			log.Errorf("[Run %d] write: %v", clientID, err)
			break
		}

		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			log.Errorf("[Run %d] read response: %v", clientID, err)
			break
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		proofGenLatency := time.Since(reqStart)

		verifyStart := time.Now()
		var commonPrefixSize int64
		var proofElements int64
		var commonPrefixElements int64
		switch protocol {
		case "optiks":
			var result optiksQueryResult
			if err := json.Unmarshal(respBody, &result); err != nil {
				log.Errorf("[Run %d] unmarshal Get response: %v", clientID, err)
			} else {
				verifyOptiksResult(clientID, user, &result, rootHash)
				commonPrefixElements = int64(len(result.CommonProofPrefix))
				for _, node := range result.CommonProofPrefix {
					commonPrefixSize += int64(len(node))
				}
				numProofs := int64(1 + len(result.VersionProofs))
				proofElements = commonPrefixElements*numProofs + int64(len(result.NextVersionNonMembershipProof))
				for _, vp := range result.VersionProofs {
					proofElements += int64(len(vp))
				}
			}
		case "samurai":
			var result samuraiQueryResult
			if err := json.Unmarshal(respBody, &result); err != nil {
				log.Errorf("[Run %d] unmarshal Samurai Get response: %v", clientID, err)
				log.Errorf("Body: %s", string(respBody))
			} else {
				verifySamuraiResult(clientID, user, &result, rootHash, precomputedData)
				proofElements = int64(len(result.MptProof)) + int64(len(result.SamuraiProofs))
			}
		}
		verifyLatency := time.Since(verifyStart)

		totalLatency := proofGenLatency + verifyLatency

		metrics.TotalRequestsCompleted++
		metrics.TotalProofGenLatency += proofGenLatency
		metrics.TotalVerifyLatency += verifyLatency
		metrics.TotalLatency += totalLatency
		metrics.TotalPayloadSize += int64(len(respBody))
		metrics.TotalCommonPrefixSize += commonPrefixSize
		metrics.TotalProofElements += proofElements
		metrics.TotalCommonPrefixElements += commonPrefixElements

		if time.Since(lastLogTime) >= time.Second {
			n := float64(metrics.TotalRequestsCompleted)
			avgGen := time.Duration(float64(metrics.TotalProofGenLatency) / n)
			avgVer := time.Duration(float64(metrics.TotalVerifyLatency) / n)
			avgTot := time.Duration(float64(metrics.TotalLatency) / n)
			avgPay := float64(metrics.TotalPayloadSize) / n
			avgPrefix := float64(metrics.TotalCommonPrefixSize) / n
			avgProofElems := float64(metrics.TotalProofElements) / n
			avgPrefixElems := float64(metrics.TotalCommonPrefixElements) / n
			log.Infof("[Run %d] reqs=%d avgProofGen=%s avgVerify=%s avgTotal=%s avgPayload=%.3fB avgCommonPrefix=%.3fB avgProofElems=%.1f avgPrefixElems=%.1f",
				clientID, metrics.TotalRequestsCompleted, avgGen, avgVer, avgTot, avgPay, avgPrefix, avgProofElems, avgPrefixElems)
			lastLogTime = time.Now()
		}
	}

	return metrics
}

// verifyOptiksResult checks that every membership proof and the non-membership
// proof in the Get response verify against the MPT root commitment.
// See KT.md § "Verification" and internal/kt/optiks_test.go.
func verifyOptiksResult(clientID int, user []byte, result *optiksQueryResult, rootHash common.Hash) {
	n := result.CurrentVersion
	prefix := result.CommonProofPrefix

	// Verify non-membership proof for version n+1.
	nonExistKey := makeTrieKey(user, n+1)
	fullNonMembershipProof := prependPrefix(prefix, result.NextVersionNonMembershipProof)
	proofDB := memorydb.New()
	for _, node := range fullNonMembershipProof {
		proofDB.Put(crypto.Keccak256(node), node)
	}
	val, err := trie.VerifyProof(rootHash, nonExistKey, proofDB)
	if err != nil {
		log.Errorf("[Run %d] non-membership proof failed for user %s version %d: %v",
			clientID, string(user), n+1, err)
	}
	if val != nil {
		log.Errorf("[Run %d] non-membership proof returned value for user %s version %d",
			clientID, string(user), n+1)
	}

	// Verify membership proofs for versions 1..n.
	for i, proof := range result.VersionProofs {
		version := uint64(i + 1)
		trieKey := makeTrieKey(user, version)

		fullProof := prependPrefix(prefix, proof)
		vProofDB := memorydb.New()
		for _, node := range fullProof {
			vProofDB.Put(crypto.Keccak256(node), node)
		}

		val, err := trie.VerifyProof(rootHash, trieKey, vProofDB)
		if err != nil {
			log.Errorf("[Run %d] membership proof failed for user %s version %d: %v",
				clientID, string(user), version, err)
		}
		if val == nil {
			log.Errorf("[Run %d] membership proof returned nil for user %s version %d",
				clientID, string(user), version)
		}
	}
}

// prependPrefix reconstructs a full proof by concatenating the common prefix
// with the per-proof suffix.
func prependPrefix(prefix [][]byte, suffix [][]byte) [][]byte {
	if len(prefix) == 0 {
		return suffix
	}
	full := make([][]byte, len(prefix)+len(suffix))
	copy(full, prefix)
	copy(full[len(prefix):], suffix)
	return full
}

// makeTrieKey builds the trie lookup key for a (user, version) pair.
// Mirrors internal/kt.makeTrieKey: Keccak256(user || bigEndian(version)).
func makeTrieKey(user []byte, version uint64) []byte {
	if len(user)+8 > 32 {
		panic(fmt.Sprintf("trie key is too long: %d", len(user)+8))
	}
	buf := make([]byte, 32)
	copy(buf, user)
	binary.BigEndian.PutUint64(buf[len(user):], version)

	return buf
}

// makeSamuraiTrieKey mirrors internal/kt.makeSamuraiTrieKey.
func makeSamuraiTrieKey(user []byte) []byte {
	buf := make([]byte, 32)
	copy(buf, user)
	return buf
}

// verifySamuraiResult verifies a Samurai KT Get response:
//  1. Verify MPT proof => extract stored commitment hash.
//  2. Deserialize top-layer BLS commitment, verify hash matches.
//  3. Deserialize historical balances and range proofs.
//  4. Delegate KZG verification to proof.VerifyNewRangeProofs.
func verifySamuraiResult(clientID int, user []byte, result *samuraiQueryResult, rootHash common.Hash, precomputedData *config.PrecomputedData) {
	if result.CurrentVersion == 0 {
		return
	}

	// 1. Verify MPT proof
	trieKey := makeSamuraiTrieKey(user)
	proofDB := memorydb.New()
	for _, node := range result.MptProof {
		proofDB.Put(crypto.Keccak256(node), node)
	}
	val, err := trie.VerifyProof(rootHash, trieKey, proofDB)
	if err != nil {
		log.Errorf("[Run %d] Samurai MPT proof failed for user %s: %v", clientID, string(user), err)
		return
	}
	if val == nil {
		log.Errorf("[Run %d] Samurai MPT proof returned nil for user %s", clientID, string(user))
		return
	}

	var decoded []byte
	if err := rlp.DecodeBytes(val, &decoded); err != nil {
		log.Errorf("[Run %d] Samurai RLP decode failed: %v", clientID, err)
		return
	}
	storedCommitmentHash := common.BytesToHash(decoded)

	// 2. Find the top-layer commitment from the proof set and verify its hash
	topLayerIdx := tree.MaxLayer
	var topCommitment gnark_kzg.Digest
	foundTop := false
	for _, sp := range result.SamuraiProofs {
		if sp.Layer == topLayerIdx {
			if _, err := topCommitment.SetBytes(sp.Commitment); err != nil {
				log.Errorf("[Run %d] Samurai: failed to unmarshal top commitment: %v", clientID, err)
				return
			}
			foundTop = true
			break
		}
	}
	if !foundTop {
		log.Errorf("[Run %d] Samurai: no top-layer commitment found in proofs", clientID)
		return
	}

	computedHash := hash.CommitmentToHash(topCommitment)
	if computedHash != storedCommitmentHash {
		log.Errorf("[Run %d] Samurai: commitment hash mismatch: MPT=%x computed=%x", clientID, storedCommitmentHash, computedHash)
		return
	}

	// 3. Deserialize historical balances and range proofs
	account := common.BytesToAddress(user)

	historicalBalances := make([]*tree.HistoricalBalance, len(result.HistoricalBalances))
	for i, hbBytes := range result.HistoricalBalances {
		hb := &tree.HistoricalBalance{}
		if err := hb.UnmarshalBinary(hbBytes); err != nil {
			log.Errorf("[Run %d] Samurai: failed to unmarshal historical balance %d: %v", clientID, i, err)
			return
		}
		historicalBalances[i] = hb
	}

	rangeProofs := make([]*proof.RangeProof, len(result.SamuraiProofs))
	for i, sp := range result.SamuraiProofs {
		var commitment gnark_kzg.Digest
		if _, err := commitment.SetBytes(sp.Commitment); err != nil {
			log.Errorf("[Run %d] Samurai: failed to unmarshal commitment: %v", clientID, err)
			return
		}
		var proofG1 bls.G1Affine
		if _, err := proofG1.SetBytes(sp.Proof); err != nil {
			log.Errorf("[Run %d] Samurai: failed to unmarshal proof: %v", clientID, err)
			return
		}
		rangeProofs[i] = &proof.RangeProof{
			Idx:                  sp.Idx,
			Layer:                sp.Layer,
			Commitment:           commitment,
			Proof:                proofG1,
			BlockRange:           sp.BlockRange,
			DependentCommitments: sp.DependentCommitments,
		}
	}

	// 4. Verify KZG range proofs via the shared proof.VerifyNewRangeProofs API
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("[Run %d] Samurai: VerifyNewRangeProofs failed for user %s: %v", clientID, string(user), r)
			}
		}()
		proof.VerifyNewRangeProofs(account, 0, result.CurrentVersion-1, rangeProofs, historicalBalances, precomputedData)
	}()
}
