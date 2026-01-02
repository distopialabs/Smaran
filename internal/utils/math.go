package utils

import (
	"math"
)

func Pow(base, exponent int) int {
	return int(math.Pow(float64(base), float64(exponent)))
}
