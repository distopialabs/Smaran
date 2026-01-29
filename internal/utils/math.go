package utils

import (
	"math"
)

// Pow returns base raised to the exponent power for int values.
func Pow(base, exponent int) int {
	return int(math.Pow(float64(base), float64(exponent)))
}

// PowUint64 returns base raised to the exponent power for uint64 values.
func PowUint64(base, exponent uint64) uint64 {
	return uint64(math.Pow(float64(base), float64(exponent)))
}
