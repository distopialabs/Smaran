package segmenttree

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/math/polynomial"
)

const (
	// StoragePath    = "/mydata/samurai/storage"
	StoragePath    = "./storage"
	PackSize       = 256
	TreeRecordSize = SegmentTreeSize * common.HashLength
)

type Storage struct {
	L1Tree map[int][]common.Hash
	L2Tree map[int][]common.Hash
	L3Tree map[int][]common.Hash
	L4Tree map[int][]common.Hash

	L1Polynomial map[int]polynomial.Polynomial
	L2Polynomial map[int]polynomial.Polynomial
	L3Polynomial map[int]polynomial.Polynomial
	L4Polynomial map[int]polynomial.Polynomial

	// LXCommitmentsV3 map[int]map[int]bls.G1Affine

	L1Commitments map[int]bls.G1Affine
	L2Commitments map[int]bls.G1Affine
	L3Commitments map[int]bls.G1Affine
	L4Commitments map[int]bls.G1Affine
}

func ensureDir(path string) error { return os.MkdirAll(path, 0o755) }

func packPath(base string, acct common.Address, layer int, kind string, packIdx int) string {
	return filepath.Join(base, "v1", acct.Hex(), fmt.Sprintf("l%d", layer), kind, fmt.Sprintf("%s_%04d.pack", kind, packIdx))
}

func serializeHashesFixed(arr []common.Hash) ([]byte, error) {
	if len(arr) != SegmentTreeSize {
		return nil, fmt.Errorf("hash array len=%d want=%d", len(arr), SegmentTreeSize)
	}
	buf := make([]byte, TreeRecordSize)
	for i := 0; i < SegmentTreeSize; i++ {
		copy(buf[i*32:(i+1)*32], arr[i][:])
	}
	return buf, nil
}

func deserializeHashesFixed(data []byte) ([]common.Hash, error) {
	if len(data) != TreeRecordSize {
		return nil, fmt.Errorf("invalid tree record size: %d want %d", len(data), TreeRecordSize)
	}
	out := make([]common.Hash, SegmentTreeSize)
	for i := 0; i < SegmentTreeSize; i++ {
		copy(out[i][:], data[i*32:(i+1)*32])
	}
	return out, nil
}

func WriteTreeSegment(base string, acct common.Address, layer, segIdx int, arr []common.Hash) error {
	packIdx := segIdx / PackSize
	segInPack := segIdx % PackSize
	offset := int64(segInPack) * int64(TreeRecordSize)
	path := packPath(base, acct, layer, "trees", packIdx)
	data, err := serializeHashesFixed(arr)
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteAt(data, offset)
	return err
}

func ReadTreeSegment(base string, acct common.Address, layer, segIdx int) ([]common.Hash, error) {
	packIdx := segIdx / PackSize
	segInPack := segIdx % PackSize
	offset := int64(segInPack) * int64(TreeRecordSize)
	path := packPath(base, acct, layer, "trees", packIdx)

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, TreeRecordSize)
	n, err := f.ReadAt(buf, offset)
	if err != nil {
		if err == io.EOF && n == TreeRecordSize {
			// exactly filled
		} else {
			return nil, err
		}
	}
	if n != TreeRecordSize {
		return nil, fmt.Errorf("short read: got %d bytes, want %d", n, TreeRecordSize)
	}
	return deserializeHashesFixed(buf)
}
