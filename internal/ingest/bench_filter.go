package ingest

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// HotAccountFilter holds a set of "hot" accounts that should be included in a
// benchmark run. Membership is tested via a hash-map keyed by address, giving
// O(1) lookup per update in the producer.
type HotAccountFilter struct {
	addrs map[common.Address]struct{}
}

// LoadHotAccountFilter reads the top numUsers addresses from a CSV file that is
// sorted by update count descending. Expected header: Address,UpdateCount.
// Only the Address column is used; the file is read line-by-line so we never
// load more than numUsers rows into memory even if the file has millions.
func LoadHotAccountFilter(csvPath string, numUsers int) (*HotAccountFilter, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("open hot accounts file: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReaderSize(f, 256*1024))

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	addrCol := -1
	for i, h := range header {
		if strings.TrimSpace(strings.ToLower(h)) == "address" {
			addrCol = i
			break
		}
	}
	if addrCol < 0 {
		return nil, fmt.Errorf("CSV missing 'Address' column; header: %v", header)
	}

	addrs := make(map[common.Address]struct{}, numUsers)
	for loaded := 0; loaded < numUsers; loaded++ {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row %d: %w", loaded+1, err)
		}
		addr := common.HexToAddress(strings.TrimSpace(record[addrCol]))
		addrs[addr] = struct{}{}
	}

	return &HotAccountFilter{addrs: addrs}, nil
}

// Contains returns true if addr is in the selected hot-account set.
func (f *HotAccountFilter) Contains(addr common.Address) bool {
	_, ok := f.addrs[addr]
	return ok
}

// Size returns the number of addresses in the filter.
func (f *HotAccountFilter) Size() int {
	return len(f.addrs)
}
