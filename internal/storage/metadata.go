package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Metadata file name
const MetadataFileName = "metadata.json"

// Metadata holds the state of the Samurai indexing process
type Metadata struct {
	LastProcessedBlock uint64 `json:"last_processed_block"`
}

// SaveMetadata saves the metadata to the specified directory
func SaveMetadata(dir string, meta Metadata) error {
	path := filepath.Join(dir, MetadataFileName)

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}
	return nil
}

// LoadMetadata loads the metadata from the specified directory
func LoadMetadata(dir string) (Metadata, error) {
	path := filepath.Join(dir, MetadataFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, nil // Return empty metadata if file doesn't exist
		}
		return Metadata{}, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return meta, nil
}

func GetLastProcessedBlockNumber(dbDir string) (uint64, error) {
	meta, err := LoadMetadata(dbDir)
	if err != nil {
		return 0, err
	}
	return meta.LastProcessedBlock, nil
}

func SetLastProcessedBlockNumber(dbDir string, blockNumber uint64) error {
	meta := Metadata{
		LastProcessedBlock: blockNumber,
	}
	return SaveMetadata(dbDir, meta)

}
