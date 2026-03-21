// Package proof implements Verkle tree proof generation, verification,
// and a JSON wrapper conforming to the Verkle-first proof format.
package proof

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/nepal80m/samurai/internal/verkle/keys"
	verkle "github.com/ethereum/go-verkle"
)

// SerializeCommitment serializes a Verkle tree root commitment point
// to compressed bytes suitable for use with verkle.Verify.
func SerializeCommitment(root verkle.VerkleNode) []byte {
	point := root.Commitment()
	if point == nil {
		return nil
	}
	b := point.Bytes()
	return b[:]
}

// ProofResult is the Verkle-first JSON wrapper for a balance proof.
type ProofResult struct {
	Address     string          `json:"address"`
	StateRoot   string          `json:"stateRoot"`
	Key         string          `json:"key"`
	Exists      bool            `json:"exists"`
	Balance     string          `json:"balance"`
	VerkleProof json.RawMessage `json:"verkleProof"`
	StateDiff   json.RawMessage `json:"stateDiff"`
}

// Metrics holds timing and size metrics for proof operations.
type Metrics struct {
	ProofGenTimeNs      int64 `json:"proof_gen_time_ns"`
	JSONMarshalTimeNs   int64 `json:"json_marshal_time_ns"`
	ProofJSONBytesLen   int   `json:"proof_json_bytes_len"`
	ProofPayloadBytesLen int  `json:"proof_payload_bytes_len"`
	VerifyTimeNs        int64 `json:"verify_time_ns,omitempty"`
}

// GenerateProof generates a Verkle multiproof for the basic-data key
// of the given address and returns a ProofResult and Metrics.
func GenerateProof(
	root verkle.VerkleNode,
	address [20]byte,
	rootCommitment []byte,
	resolver verkle.NodeResolverFn,
) (*ProofResult, *Metrics, error) {
	treeKey := keys.GetTreeKeyForBasicData(address)
	keySlice := treeKey[:]

	// Check existence
	val, err := root.Get(keySlice, resolver)
	exists := err == nil && val != nil

	var balance *big.Int
	if exists {
		var val32 [32]byte
		copy(val32[:], val)
		balance = keys.UnpackBalance(val32)
	} else {
		balance = new(big.Int)
	}

	// Generate proof with timing
	proofStart := time.Now()
	proof, _, _, _, err := verkle.MakeVerkleMultiProof(root, root, [][]byte{keySlice}, resolver)
	if err != nil {
		return nil, nil, fmt.Errorf("MakeVerkleMultiProof: %w", err)
	}
	proofGenTime := time.Since(proofStart)

	// Serialize proof
	vp, sd, err := verkle.SerializeProof(proof)
	if err != nil {
		return nil, nil, fmt.Errorf("SerializeProof: %w", err)
	}

	// Compute payload size from the serialized proof structure
	payloadSize := computePayloadSize(vp, sd)

	// Marshal to JSON with timing
	marshalStart := time.Now()
	vpJSON, err := json.Marshal(vp)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal VerkleProof: %w", err)
	}
	sdJSON, err := json.Marshal(sd)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal StateDiff: %w", err)
	}

	result := &ProofResult{
		Address:     "0x" + hex.EncodeToString(address[:]),
		StateRoot:   "0x" + hex.EncodeToString(rootCommitment),
		Key:         "0x" + hex.EncodeToString(treeKey[:]),
		Exists:      exists,
		Balance:     "0x" + balance.Text(16),
		VerkleProof: vpJSON,
		StateDiff:   sdJSON,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal result: %w", err)
	}
	marshalTime := time.Since(marshalStart)

	metrics := &Metrics{
		ProofGenTimeNs:       proofGenTime.Nanoseconds(),
		JSONMarshalTimeNs:    marshalTime.Nanoseconds(),
		ProofJSONBytesLen:    len(resultJSON),
		ProofPayloadBytesLen: payloadSize,
	}

	return result, metrics, nil
}

// computePayloadSize estimates the canonical binary size of the rust-verkle
// serialization format as documented in SerializeProof:
//   - len(proof-of-absence stems) + stems themselves (31 bytes each)
//   - len(depths) + depth/ext status pairs (1 byte each)
//   - len(commitments) + commitments (32 bytes each compressed)
//   - multipoint proof (fixed 576 bytes for IPA: 8 rounds * 2 * 32 + 2 * 32)
//   - keys and values from StateDiff
func computePayloadSize(vp *verkle.VerkleProof, sd verkle.StateDiff) int {
	size := 0

	// Proof of absence stems: 4 bytes length + 31 bytes each
	size += 4 + len(vp.OtherStems)*31

	// Depth/extension present: 4 bytes length + 1 byte each
	size += 4 + len(vp.DepthExtensionPresent)

	// Commitments by path: 4 bytes length + 32 bytes each
	size += 4 + len(vp.CommitmentsByPath)*32

	// D commitment: 32 bytes
	size += 32

	// IPA proof: 8 rounds * 2 points * 32 bytes + 2 scalars * 32 bytes = 576 bytes
	size += 576

	// StateDiff: for each stem diff
	for _, ssd := range sd {
		// stem: 31 bytes
		size += 31
		// suffix diffs count: 4 bytes
		size += 4
		for _, suffDiff := range ssd.SuffixDiffs {
			// suffix: 1 byte
			size += 1
			// current value: 1 byte flag + optional 32 bytes
			size += 1
			if suffDiff.CurrentValue != nil {
				size += 32
			}
			// new value: 1 byte flag + optional 32 bytes
			size += 1
			if suffDiff.NewValue != nil {
				size += 32
			}
		}
	}

	return size
}

// VerifyProof verifies a Verkle proof against a root commitment.
// Uses the same root for both pre-state and post-state because these are
// read-only balance proofs with no state transitions.
func VerifyProof(rootBytes []byte, vp *verkle.VerkleProof, sd verkle.StateDiff) error {
	return verkle.Verify(vp, rootBytes, rootBytes, sd)
}

// VerifyAndExtract deserializes a proof, verifies it, and extracts
// the balance for the given address.
func VerifyAndExtract(
	rootBytes []byte,
	vp *verkle.VerkleProof,
	sd verkle.StateDiff,
	address [20]byte,
) (exists bool, balance *big.Int, err error) {
	// Verify the proof
	if err := verkle.Verify(vp, rootBytes, rootBytes, sd); err != nil {
		return false, nil, fmt.Errorf("verification failed: %w", err)
	}

	// Extract value for the basic-data key from state diff
	treeKey := keys.GetTreeKeyForBasicData(address)
	stem := treeKey[:31]
	suffix := treeKey[31]

	for _, ssd := range sd {
		if bytes.Equal(ssd.Stem[:], stem) {
			for _, suffDiff := range ssd.SuffixDiffs {
				if suffDiff.Suffix == suffix {
					if suffDiff.CurrentValue != nil {
						balance = keys.UnpackBalance(*suffDiff.CurrentValue)
						return true, balance, nil
					}
					return false, new(big.Int), nil
				}
			}
		}
	}

	return false, new(big.Int), nil
}
