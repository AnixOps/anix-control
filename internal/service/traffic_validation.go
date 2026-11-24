package service

import (
	"errors"
	"math"
	"math/bits"
)

var ErrNegativeTraffic = errors.New("traffic values must be non-negative")

// ErrTrafficOverflow is a traffic amount that does not fit the int64
// counters once multiplied by the node's rate or added up.
var ErrTrafficOverflow = errors.New("traffic exceeds the counter range")

func ValidateTrafficDelta(upload, download int64) error {
	if upload < 0 || download < 0 {
		return ErrNegativeTraffic
	}
	return nil
}

// scaleTraffic is bytes times rate, the rate taken to 1/1000 and the result
// rounded down, in integer arithmetic: exact for every int64 and never
// wrapping (CT-006).
func scaleTraffic(bytes int64, rate float64) (int64, error) {
	milli := math.Round(rate * 1000)
	if milli >= math.MaxUint64 {
		return 0, ErrTrafficOverflow
	}
	hi, lo := bits.Mul64(uint64(bytes), uint64(milli)) // #nosec G115 -- bytes is validated non-negative.
	if hi >= 1000 {
		return 0, ErrTrafficOverflow
	}
	scaled, _ := bits.Div64(hi, lo, 1000)
	if scaled > math.MaxInt64 {
		return 0, ErrTrafficOverflow
	}
	return int64(scaled), nil
}

// addTraffic is a + b, refusing a sum beyond int64.
func addTraffic(a, b int64) (int64, error) {
	if b > math.MaxInt64-a {
		return 0, ErrTrafficOverflow
	}
	return a + b, nil
}
