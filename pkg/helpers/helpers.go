// Package helpers provides shared numeric and slice utilities.
package helpers

// AbsInt returns the absolute value of a, subject to integer overflow.
func AbsInt(a int) int {
	if a < 0 {
		return a * -1
	}

	return a
}

// FilterFromSlice retains matching elements in order, reusing the input's backing array.
func FilterFromSlice[T any](slice []T, keep func(T) bool) []T {
	nIndex := 0 // next write position

	for _, v := range slice { // read position
		if keep(v) {
			slice[nIndex] = v // overwrite; OK even when nIndex==read index
			nIndex++
		}
	}

	return slice[:nIndex] // truncate tail
}

// RoundUp adds one half to f and truncates the result toward zero.
func RoundUp(f float32) int {
	const half = 0.5

	return int(f + half)
}

// Delete removes the element at i, reusing the input's backing array.
func Delete[T any](s []T, i int) []T {
	return append(s[:i], s[i+1:]...)
}
