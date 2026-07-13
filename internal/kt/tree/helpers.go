package tree

import (
	"encoding/json"
	"time"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/nepal80m/samurai/internal/logging"
)

var log = logging.GetLogger("tree")

// logBlockedTime logs a warning if a named operation is blocked for longer than d.
func logBlockedTime(name string, d time.Duration) chan struct{} {
	start := time.Now()
	ticker := time.NewTicker(d)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				log.Warningf("%s blocked for %v", name, time.Since(start))
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	return quit
}

// MarshalG1AffineMap serializes a map of G1Affine points to JSON bytes.
func MarshalG1AffineMap(g1AffineMap map[int]bls.G1Affine) ([]byte, error) {
	g1affinebytesMap := make(map[int][]byte)
	for k, v := range g1AffineMap {
		g1affinebytesMap[k] = v.Marshal()
	}
	return json.Marshal(g1affinebytesMap)
}

// UnmarshalG1AffineMap deserializes JSON bytes to a map of G1Affine points.
func UnmarshalG1AffineMap(data []byte) (map[int]bls.G1Affine, error) {
	var g1affinebytesMap map[int][]byte
	err := json.Unmarshal(data, &g1affinebytesMap)
	if err != nil {
		return nil, err
	}
	g1AffineMap := make(map[int]bls.G1Affine)
	for k, v := range g1affinebytesMap {
		var tempG1Affine bls.G1Affine
		tempG1Affine.Unmarshal(v)
		g1AffineMap[k] = tempG1Affine
	}
	return g1AffineMap, nil
}

// GetAncestors returns ancestor node indices from nodeIdx up to the root.
func GetAncestors(nodeIdx uint64) []uint64 {
	ancestors := []uint64{}
	for nodeIdx > 0 {
		parentNodeIdx := GetParent(nodeIdx)
		ancestors = append(ancestors, parentNodeIdx)
		nodeIdx = parentNodeIdx
	}
	return ancestors
}

// GetParent returns the parent node index in a binary tree.
func GetParent(nodeIdx uint64) uint64 {
	if nodeIdx == 0 {
		panic("root has no parent.")
	}
	if nodeIdx&1 == 0 {
		return (nodeIdx - 2) / 2
	}
	return (nodeIdx - 1) / 2
}

// GetLeftChild returns the left child node index.
func GetLeftChild(nodeIdx int) int {
	return 2*nodeIdx + 1
}

// GetRightChild returns the right child node index.
func GetRightChild(nodeIdx int) int {
	return 2*nodeIdx + 2
}

// GetSibling returns the sibling node index.
func GetSibling(nodeIdx int) int {
	if nodeIdx == 0 {
		return 0
	}
	if nodeIdx&1 == 0 {
		return nodeIdx - 1
	}
	return nodeIdx + 1
}
