package dataset

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type DatasetWriter struct {
	Dir         string
	SegmentSize uint32 // e.g., 100_000
	cur         *segmentWriter
}

func NewDatasetWriter(dir string, segmentSize uint32) *DatasetWriter {
	return &DatasetWriter{Dir: dir, SegmentSize: segmentSize}
}

// AppendBlock appends one block (must be in strictly increasing global order).
func (dw *DatasetWriter) AppendBlock(block uint32, entries []Entry) error {
	base, end := segmentBounds(block, dw.SegmentSize)

	// Rotate/open segment if needed
	if dw.cur == nil || dw.cur.base != base {
		if dw.cur != nil {
			if err := dw.cur.close(); err != nil {
				return err
			}
		}
		sw, err := openSegmentWriter(dw.Dir, base, end)
		if err != nil {
			return err
		}
		dw.cur = sw
	}

	return dw.cur.appendBlock(block, entries)
}

func (dw *DatasetWriter) Close() error {
	if dw.cur == nil {
		return nil
	}
	err := dw.cur.close()
	dw.cur = nil
	return err
}

type segmentWriter struct {
	base, end uint32
	dat       *os.File
	datBuf    *bufio.Writer
	idx       *os.File // append-only offsets
	nextBlock uint32
}

func openSegmentWriter(dir string, base, end uint32) (*segmentWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	datPath := filepath.Join(dir, segmentName(base, end, "dat"))
	idxPath := filepath.Join(dir, segmentName(base, end, "idx"))

	dat, err := os.Create(datPath)
	if err != nil {
		return nil, err
	}

	idx, err := os.Create(idxPath)
	if err != nil {
		_ = dat.Close()
		return nil, err
	}

	sw := &segmentWriter{
		base: base, end: end,
		dat: dat, datBuf: bufio.NewWriterSize(dat, 1<<20),
		idx:       idx,
		nextBlock: base,
	}

	// Write initial offset[0] = 0
	var zero [8]byte
	if _, err := sw.idx.Write(zero[:]); err != nil {
		_ = sw.dat.Close()
		_ = sw.idx.Close()
		return nil, err
	}
	return sw, nil
}

func (sw *segmentWriter) appendBlock(block uint32, entries []Entry) error {
	if block < sw.base || block > sw.end {
		return fmt.Errorf("block %d out of segment range [%d..%d]", block, sw.base, sw.end)
	}
	if block != sw.nextBlock {
		return fmt.Errorf("non-sequential append: got %d, expected %d", block, sw.nextBlock)
	}

	// Start offset is current data length
	// (We don’t store it—only the new end offset is appended to .idx)
	// Write K
	K := uint32(len(entries))
	if err := binary.Write(sw.datBuf, binary.LittleEndian, K); err != nil {
		return err
	}

	// Entries: address(20) + uvarint(len) + balance
	var vbuf [10]byte
	for i := range entries {
		if _, err := sw.datBuf.Write(entries[i].Address[:]); err != nil {
			return err
		}
		n := binary.PutUvarint(vbuf[:], uint64(len(entries[i].Balance)))
		if _, err := sw.datBuf.Write(vbuf[:n]); err != nil {
			return err
		}
		if len(entries[i].Balance) > 0 {
			if _, err := sw.datBuf.Write(entries[i].Balance); err != nil {
				return err
			}
		}
	}

	// Flush to ensure file size is accurate
	if err := sw.datBuf.Flush(); err != nil {
		return err
	}

	// Append new end offset to .idx
	endOff, err := sw.dat.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	var obuf [8]byte
	binary.LittleEndian.PutUint64(obuf[:], uint64(endOff))
	if _, err := sw.idx.Write(obuf[:]); err != nil {
		return err
	}

	sw.nextBlock++
	return nil
}

func (sw *segmentWriter) close() error {
	if sw.datBuf != nil {
		if err := sw.datBuf.Flush(); err != nil {
			_ = sw.dat.Close()
			_ = sw.idx.Close()
			return err
		}
	}
	if sw.dat != nil {
		_ = sw.dat.Sync()
		_ = sw.dat.Close()
		sw.dat = nil
	}
	if sw.idx != nil {
		_ = sw.idx.Sync()
		_ = sw.idx.Close()
		sw.idx = nil
	}
	return nil
}
