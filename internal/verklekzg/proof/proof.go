// Package proof implements KZG path-proof generation and verification for
// the Verkle-KZG trie. A proof consists of one KZG opening per trie level
// along the path from root to the queried leaf slot.
package proof

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/nepal80m/samurai/internal/crypto/hash"
	"github.com/nepal80m/samurai/internal/verkle/keys"
	"github.com/nepal80m/samurai/internal/verklekzg/tree"
)

// PathOpening is a single KZG opening proof along the Verkle path.
type PathOpening struct {
	Commitment gnark_kzg.Digest  `json:"commitment"`
	Proof      gnark_kzg.Digest  `json:"proof"`
	Point      fr.Element        `json:"point"`
	ClaimedVal fr.Element        `json:"claimedVal"`
}

// VerkleKZGProof is the full proof for a key lookup.
type VerkleKZGProof struct {
	Openings []PathOpening `json:"openings"`
}

// ProofResult is the JSON wrapper returned by GenerateProof.
type ProofResult struct {
	Address   string          `json:"address"`
	StateRoot string          `json:"stateRoot"`
	Key       string          `json:"key"`
	Exists    bool            `json:"exists"`
	Balance   string          `json:"balance"`
	Proof     json.RawMessage `json:"proof"`
}

// Metrics holds timing and size metrics.
type Metrics struct {
	ProofGenTimeNs      int64 `json:"proof_gen_time_ns"`
	JSONMarshalTimeNs   int64 `json:"json_marshal_time_ns"`
	ProofJSONBytesLen   int   `json:"proof_json_bytes_len"`
	ProofPayloadBytesLen int  `json:"proof_payload_bytes_len"`
	VerifyTimeNs        int64 `json:"verify_time_ns,omitempty"`
}

// SerializeCommitment returns the compressed bytes of the root commitment.
func SerializeCommitment(root tree.VerkleNode) []byte {
	return tree.CommitmentBytes(root.Commitment())
}

// GenerateProof generates a KZG Verkle path proof for the basic-data key of
// the given address.
func GenerateProof(
	root *tree.InternalNode,
	address [20]byte,
	rootCommitment []byte,
	resolver tree.NodeResolverFn,
	cfg *tree.TreeConfig,
) (*ProofResult, *Metrics, error) {
	treeKey := keys.GetTreeKeyForBasicData(address)
	keySlice := treeKey[:]
	stem := keySlice[:31]
	suffix := keySlice[31]

	val, getErr := root.Get(keySlice, resolver)
	exists := getErr == nil && val != nil

	var balance *big.Int
	if exists {
		var val32 [32]byte
		copy(val32[:], val)
		balance = keys.UnpackBalance(val32)
	} else {
		balance = new(big.Int)
	}

	proofStart := time.Now()

	var openings []PathOpening
	var current tree.VerkleNode = root

	// Walk down internal nodes, generating one KZG opening per level.
	for depth := byte(0); depth < 31; depth++ {
		inode, ok := current.(*tree.InternalNode)
		if !ok {
			break
		}

		childIdx := stem[depth]
		point := cfg.OmegaPow(int(childIdx))

		child := inode.Child(childIdx)
		if child == nil {
			// Non-membership: the path ends here.
			opening, err := generateOpening(inode, childIdx, cfg)
			if err != nil {
				return nil, nil, fmt.Errorf("depth %d: %w", depth, err)
			}
			openings = append(openings, opening)
			break
		}

		childHash := hash.CommitmentToHash(child.Commitment())
		var claimedVal fr.Element
		claimedVal.SetBigInt(childHash.Big())

		poly := buildInternalPoly(inode, cfg)
		proof, err := gnark_kzg.Open(poly, point, cfg.SRS.Inner.Pk)
		if err != nil {
			return nil, nil, fmt.Errorf("depth %d: KZG open: %w", depth, err)
		}

		openings = append(openings, PathOpening{
			Commitment: inode.Commitment(),
			Proof:      proof.H,
			Point:      point,
			ClaimedVal: claimedVal,
		})

		current = child
	}

	// If we reached a leaf, generate an opening for the value slot.
	if leaf, ok := current.(*tree.LeafNode); ok {
		point := cfg.OmegaPow(int(suffix))

		poly := buildLeafPoly(leaf, cfg)
		proof, err := gnark_kzg.Open(poly, point, cfg.SRS.Inner.Pk)
		if err != nil {
			return nil, nil, fmt.Errorf("leaf KZG open: %w", err)
		}

		var claimedVal fr.Element
		if leaf.Value(suffix) != nil {
			v := leaf.Value(suffix)
			claimedVal.SetBytes(v[:])
		}

		openings = append(openings, PathOpening{
			Commitment: leaf.Commitment(),
			Proof:      proof.H,
			Point:      point,
			ClaimedVal: claimedVal,
		})
	}

	proofGenTime := time.Since(proofStart)

	vkProof := VerkleKZGProof{Openings: openings}

	marshalStart := time.Now()
	proofJSON, err := json.Marshal(vkProof)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal proof: %w", err)
	}

	result := &ProofResult{
		Address:   "0x" + hex.EncodeToString(address[:]),
		StateRoot: "0x" + hex.EncodeToString(rootCommitment),
		Key:       "0x" + hex.EncodeToString(treeKey[:]),
		Exists:    exists,
		Balance:   "0x" + balance.Text(16),
		Proof:     proofJSON,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal result: %w", err)
	}
	marshalTime := time.Since(marshalStart)

	// Payload: each opening = 48 (commit) + 48 (proof) + 32 (point) + 32 (val) = 160 bytes
	payloadSize := len(openings) * 160

	metrics := &Metrics{
		ProofGenTimeNs:       proofGenTime.Nanoseconds(),
		JSONMarshalTimeNs:    marshalTime.Nanoseconds(),
		ProofJSONBytesLen:    len(resultJSON),
		ProofPayloadBytesLen: payloadSize,
	}

	return result, metrics, nil
}

// VerifyProof verifies a VerkleKZGProof. It checks that each opening is valid
// and that the commitments chain correctly from root to leaf.
func VerifyProof(rootCommitment []byte, vkProof *VerkleKZGProof, cfg *tree.TreeConfig) error {
	if len(vkProof.Openings) == 0 {
		return fmt.Errorf("empty proof")
	}

	// Verify the root commitment matches.
	rootBytes := tree.CommitmentBytes(vkProof.Openings[0].Commitment)
	if len(rootCommitment) != len(rootBytes) {
		return fmt.Errorf("root commitment length mismatch")
	}
	for i := range rootBytes {
		if rootBytes[i] != rootCommitment[i] {
			return fmt.Errorf("root commitment mismatch")
		}
	}

	for i, op := range vkProof.Openings {
		claim := gnark_kzg.OpeningProof{
			H:            op.Proof,
			ClaimedValue: op.ClaimedVal,
		}
		if err := gnark_kzg.Verify(&op.Commitment, &claim, op.Point, cfg.SRS.Inner.Vk); err != nil {
			return fmt.Errorf("opening %d: KZG verify failed: %w", i, err)
		}

		// For non-leaf openings, verify the claimed value chains to the next
		// level's commitment hash.
		if i < len(vkProof.Openings)-1 {
			nextCommit := vkProof.Openings[i+1].Commitment
			expectedHash := hash.CommitmentToHash(nextCommit)
			var expectedFr fr.Element
			expectedFr.SetBigInt(expectedHash.Big())
			if !op.ClaimedVal.Equal(&expectedFr) {
				return fmt.Errorf("opening %d: commitment chain mismatch", i)
			}
		}
	}

	return nil
}

// VerifyAndExtract verifies the proof and extracts the balance.
func VerifyAndExtract(
	rootCommitment []byte,
	vkProof *VerkleKZGProof,
	address [20]byte,
	cfg *tree.TreeConfig,
) (exists bool, balance *big.Int, err error) {
	if err := VerifyProof(rootCommitment, vkProof, cfg); err != nil {
		return false, nil, err
	}

	// The last opening's claimed value is the leaf slot value.
	lastOp := vkProof.Openings[len(vkProof.Openings)-1]
	if lastOp.ClaimedVal.IsZero() {
		return false, new(big.Int), nil
	}

	var valBig big.Int
	lastOp.ClaimedVal.BigInt(&valBig)
	valBytes := valBig.Bytes()

	var val32 [32]byte
	if len(valBytes) <= 32 {
		copy(val32[32-len(valBytes):], valBytes)
	}
	balance = keys.UnpackBalance(val32)
	return true, balance, nil
}

// ---------------------------------------------------------------------------
// Polynomial reconstruction helpers
// ---------------------------------------------------------------------------

func generateOpening(inode *tree.InternalNode, childIdx byte, cfg *tree.TreeConfig) (PathOpening, error) {
	point := cfg.OmegaPow(int(childIdx))
	poly := buildInternalPoly(inode, cfg)
	proof, err := gnark_kzg.Open(poly, point, cfg.SRS.Inner.Pk)
	if err != nil {
		return PathOpening{}, err
	}
	var zero fr.Element
	return PathOpening{
		Commitment: inode.Commitment(),
		Proof:      proof.H,
		Point:      point,
		ClaimedVal: zero,
	}, nil
}

// buildInternalPoly reconstructs the evaluation polynomial for an internal
// node: poly[i] = Fr(hash(child_i.commitment)) for occupied slots, 0 otherwise.
// Returned as coefficient form via inverse FFT would be ideal, but for KZG
// opening we need the polynomial in coefficient form. We use Lagrange
// interpolation via the barycentric formula.
func buildInternalPoly(inode *tree.InternalNode, cfg *tree.TreeConfig) []fr.Element {
	var evals [tree.DomainSize]fr.Element
	children := inode.Children()
	for i := 0; i < tree.Width; i++ {
		if children[i] == nil {
			continue
		}
		h := hash.CommitmentToHash(children[i].Commitment())
		evals[i].SetBigInt(h.Big())
	}
	return evalsToCoeffs(evals[:], cfg)
}

func buildLeafPoly(leaf *tree.LeafNode, cfg *tree.TreeConfig) []fr.Element {
	var evals [tree.DomainSize]fr.Element
	for i := 0; i < tree.Width; i++ {
		if i == tree.Width-1 {
			stem := leaf.Stem()
			stemHash := hash.BytesToHash(stem[:])
			evals[i].SetBigInt(stemHash.Big())
		} else if v := leaf.Value(byte(i)); v != nil {
			evals[i].SetBytes(v[:])
		}
	}
	return evalsToCoeffs(evals[:], cfg)
}

// evalsToCoeffs converts evaluation-form polynomial (values at omega^i) to
// coefficient form using barycentric interpolation with the precomputed
// vanishing polynomial and weights.
func evalsToCoeffs(evals []fr.Element, cfg *tree.TreeConfig) []fr.Element {
	n := tree.DomainSize
	coeffs := make([]fr.Element, n)
	quot := make([]fr.Element, n)

	for i := 0; i < n; i++ {
		if evals[i].IsZero() {
			continue
		}
		omega_i := cfg.OmegaPow(i)
		syntheticDivide(quot, cfg.V, &omega_i)

		var scale fr.Element
		scale.Mul(&evals[i], &cfg.Weights[i])

		for k := 0; k < n; k++ {
			var t fr.Element
			t.Mul(&quot[k], &scale)
			coeffs[k].Add(&coeffs[k], &t)
		}
	}
	return coeffs
}

// syntheticDivide computes quotient = P / (X - a).
func syntheticDivide(quot []fr.Element, P []fr.Element, a *fr.Element) {
	deg := len(P) - 1
	if len(quot) != deg {
		panic(fmt.Sprintf("syntheticDivide: quot len %d != deg %d", len(quot), deg))
	}
	quot[deg-1] = P[deg]
	for i := deg - 2; i >= 0; i-- {
		var tmp fr.Element
		tmp.Mul(&quot[i+1], a)
		quot[i].Add(&P[i+1], &tmp)
	}
}
