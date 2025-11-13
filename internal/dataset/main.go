package dataset

const DATASET_DIR = "data/modified_accounts"
const SEGMENT_SIZE = 100_000

type Entry struct {
	Address [20]byte // EVM address
	// Balance is big-endian minimal (0..32 bytes).
	Balance []byte
}
