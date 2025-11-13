package dataset

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type DatasetReader struct {
	Dir         string
	SegmentSize uint32
	cur         *segmentReader // simple single-segment cache
}

func NewDatasetReader(dir string, segmentSize uint32) *DatasetReader {
	return &DatasetReader{Dir: dir, SegmentSize: segmentSize}
}

func (dr *DatasetReader) GetBlock(block uint32) ([]Entry, error) {
	sr, err := dr.ensureSegment(block)
	if err != nil {
		return nil, err
	}
	return sr.getBlock(block)
}

func (dr *DatasetReader) IterateRange(a, b uint32, fn func(n uint32, entries []Entry) error) error {
	if b < a {
		return nil
	}
	n := a
	for n <= b {
		sr, err := dr.ensureSegment(n)
		if err != nil {
			return err
		}

		segStop := sr.end
		if segStop > b {
			segStop = b
		}
		// also cap by last written block in this segment
		if sr.lastWritten < segStop {
			segStop = sr.lastWritten
		}

		for ; n <= segStop; n++ {
			es, err := sr.getBlock(n)
			if err != nil {
				return err
			}
			if err := fn(n, es); err != nil {
				return err
			}
		}

		// if the requested range goes past what’s built in this segment, advance to next segment
		if n <= b && segStop < sr.end {
			n = sr.end + 1
		}
	}
	return nil
}

func (dr *DatasetReader) Close() error {
	if dr.cur == nil {
		return nil
	}
	err := dr.cur.close()
	dr.cur = nil
	return err
}

type segmentReader struct {
	base, end   uint32
	dat         *os.File
	offsets     []uint64 // length = writtenBlocks+1
	lastWritten uint32   // absolute block number of last written block in this segment; if none, base-1
}

func openSegmentReader(dir string, base, end uint32) (*segmentReader, error) {
	datPath := filepath.Join(dir, segmentName(base, end, "dat"))
	idxPath := filepath.Join(dir, segmentName(base, end, "idx"))

	dat, err := os.Open(datPath)
	if err != nil {
		return nil, err
	}

	idxBytes, err := os.ReadFile(idxPath)
	if err != nil {
		_ = dat.Close()
		return nil, err
	}

	if len(idxBytes)%8 != 0 {
		_ = dat.Close()
		return nil, fmt.Errorf("index file corrupt: size %d not multiple of 8", len(idxBytes))
	}

	// Number of written blocks is (len/8 - 1). Offsets always include the leading offset[0].
	nOffsets := len(idxBytes) / 8
	if nOffsets < 1 {
		_ = dat.Close()
		return nil, errors.New("index file missing initial offset")
	}

	offsets := make([]uint64, nOffsets)
	for i := 0; i < nOffsets; i++ {
		offsets[i] = binary.LittleEndian.Uint64(idxBytes[i*8 : i*8+8])
	}

	var lastWritten uint32
	if nOffsets == 1 {
		// No blocks written yet
		lastWritten = base - 1
	} else {
		lastWritten = base + uint32(nOffsets-2) // -1 for leading offset, -1 to get 0-based block index
	}

	return &segmentReader{
		base: base, end: end,
		dat: dat, offsets: offsets,
		lastWritten: lastWritten,
	}, nil
}

func (sr *segmentReader) getBlock(block uint32) ([]Entry, error) {
	if block < sr.base || block > sr.end {
		return nil, fmt.Errorf("block %d out of segment range [%d..%d]", block, sr.base, sr.end)
	}
	if block > sr.lastWritten {
		return nil, fmt.Errorf("block %d not built yet (last built %d)", block, sr.lastWritten)
	}

	i := int(block - sr.base)
	start := int64(sr.offsets[i])
	end := int64(sr.offsets[i+1])
	if end < start {
		return nil, errors.New("corrupt index: end < start")
	}

	if end == start {
		// Block is present and empty (no modified accounts)
		return nil, nil
	}

	size := end - start
	buf := make([]byte, size)
	n, err := sr.dat.ReadAt(buf, start)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n != int(size) {
		return nil, io.ErrUnexpectedEOF
	}

	// Decode
	pos := 0
	if pos+4 > len(buf) {
		return nil, io.ErrUnexpectedEOF
	}
	K := binary.LittleEndian.Uint32(buf[pos : pos+4])
	pos += 4

	out := make([]Entry, 0, K)
	for j := 0; j < int(K); j++ {
		if pos+20 > len(buf) {
			return nil, io.ErrUnexpectedEOF
		}
		var e Entry
		copy(e.Address[:], buf[pos:pos+20])
		pos += 20

		blen, n := binary.Uvarint(buf[pos:])
		if n <= 0 {
			return nil, io.ErrUnexpectedEOF
		}
		pos += n

		if blen > 0 {
			if pos+int(blen) > len(buf) {
				return nil, io.ErrUnexpectedEOF
			}
			e.Balance = append(e.Balance[:0], buf[pos:pos+int(blen)]...)
			pos += int(blen)
		} else {
			e.Balance = e.Balance[:0]
		}
		out = append(out, e)
	}
	return out, nil
}

func (sr *segmentReader) close() error {
	if sr.dat != nil {
		_ = sr.dat.Close()
		sr.dat = nil
	}
	return nil
}
