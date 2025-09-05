package math

import "math"

// func Pow[T ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](base, exponent T) T {
// 	return T(math.Pow(float64(base), float64(exponent)))
// }

func Pow(base, exponent uint64) uint64 {
	return uint64(math.Pow(float64(base), float64(exponent)))
}
