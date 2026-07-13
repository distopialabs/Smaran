package benchutil

import (
	"strconv"
	"time"
)

const benchLogChanSize = 100_000

// BenchLogHeader is the CSV header for server-side bench logs.
var BenchLogHeader = []string{"start_at_ns", "completed_at_ns"}

// BenchRecord holds timestamps for a single request.
type BenchRecord struct {
	StartNs     int64
	CompletedNs int64
}

// BenchLogger is a channel-based async CSV logger for server-side benchmarking.
// Handlers send records to a buffered channel; a single goroutine drains it
// and writes to a 1MB-buffered CSV file. The channel is large enough (100k)
// that it never blocks in practice.
type BenchLogger struct {
	ch   chan BenchRecord
	csv  *BenchCSVWriter
	done chan struct{}
}

// NewBenchLogger creates a BenchLogger that writes to the given CSV path.
func NewBenchLogger(path string) (*BenchLogger, error) {
	csv, err := NewBenchCSVWriter(path, BenchLogHeader)
	if err != nil {
		return nil, err
	}
	return &BenchLogger{
		ch:   make(chan BenchRecord, benchLogChanSize),
		csv:  csv,
		done: make(chan struct{}),
	}, nil
}

// Log sends a record to the writer goroutine. It blocks only if the internal
// buffer (100k entries) is full, which should not happen under normal load.
func (l *BenchLogger) Log(r BenchRecord) {
	l.ch <- r
}

// Run drains the channel and writes records to CSV. A periodic flush (every 1s)
// ensures data reaches disk even if the process is killed with SIGKILL.
// Call as a goroutine.
func (l *BenchLogger) Run() {
	defer close(l.done)

	flushTicker := time.NewTicker(time.Second)
	defer flushTicker.Stop()

	for {
		select {
		case r, ok := <-l.ch:
			if !ok {
				// Channel closed — drain any remaining records.
				l.csv.Close()
				return
			}
			_ = l.csv.WriteRow(
				strconv.FormatInt(r.StartNs, 10),
				strconv.FormatInt(r.CompletedNs, 10),
			)
		case <-flushTicker.C:
			_ = l.csv.Flush()
		}
	}
}

// Stop closes the channel, waits for all buffered records to be written,
// then closes the CSV file.
func (l *BenchLogger) Stop() {
	close(l.ch)
	<-l.done
}
