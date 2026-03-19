package dataset

const SEGMENT_SIZE = 100_000
const FIRST_BLOCK uint64 = 18908895 // first block of 2024
const LAST_BLOCK uint64 = 21525890  // last block of 2024

type Entry struct {
	Address [20]byte // EVM address
	Balance []byte   // Balance is big-endian minimal
}
