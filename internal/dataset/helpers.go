package dataset

import "fmt"

func segmentBounds(block, segSize uint32) (base, end uint32) {
	base = (block / segSize) * segSize
	end = base + segSize - 1
	return
}

func segmentName(base, end uint32, ext string) string {
	return fmt.Sprintf("blk_%08d_%08d.%s", base, end, ext)
}

func (dr *DatasetReader) ensureSegment(block uint32) (*segmentReader, error) {
	base, end := segmentBounds(block, dr.SegmentSize)
	if dr.cur != nil && dr.cur.base == base {
		return dr.cur, nil
	}
	if dr.cur != nil {
		_ = dr.cur.close()
		dr.cur = nil
	}

	sr, err := openSegmentReader(dr.Dir, base, end)
	if err != nil {
		return nil, err
	}
	dr.cur = sr
	return sr, nil
}
