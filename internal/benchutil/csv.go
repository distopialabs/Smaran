package benchutil

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
)

// IngestionCSVHeader is the standard header for ingestion benchmark CSVs.
// All three protocols (samurai, merkle, verkle) use these columns.
// Samurai appends "wait_commitments_ns" for its parallel pipeline.
var IngestionCSVHeader = []string{
	"block_num",
	"num_raw_updates",
	"num_selected_updates",
	"queued_at_ns",
	"start_at_ns",
	"completed_at_ns",
}

// BenchCSVWriter wraps a buffered CSV writer for benchmark output.
type BenchCSVWriter struct {
	file *os.File
	buf  *bufio.Writer
	w    *csv.Writer
}

// NewBenchCSVWriter creates a new CSV writer at the given path, writing the
// header as the first row.
func NewBenchCSVWriter(path string, header []string) (*BenchCSVWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create CSV %s: %w", path, err)
	}

	buf := bufio.NewWriterSize(f, 1<<20) // 1MB buffer
	w := csv.NewWriter(buf)

	if err := w.Write(header); err != nil {
		f.Close()
		return nil, fmt.Errorf("write CSV header: %w", err)
	}

	return &BenchCSVWriter{file: f, buf: buf, w: w}, nil
}

// WriteRow writes a single CSV row.
func (b *BenchCSVWriter) WriteRow(fields ...string) error {
	return b.w.Write(fields)
}

// Flush pushes buffered data to disk without closing the file.
func (b *BenchCSVWriter) Flush() error {
	b.w.Flush()
	if err := b.w.Error(); err != nil {
		return err
	}
	return b.buf.Flush()
}

// Close flushes and closes the underlying file.
func (b *BenchCSVWriter) Close() error {
	if err := b.Flush(); err != nil {
		b.file.Close()
		return err
	}
	return b.file.Close()
}
