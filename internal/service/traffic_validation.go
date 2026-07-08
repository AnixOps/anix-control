package service

import "errors"

var ErrNegativeTraffic = errors.New("traffic values must be non-negative")

func ValidateTrafficDelta(upload, download int64) error {
	if upload < 0 || download < 0 {
		return ErrNegativeTraffic
	}
	return nil
}
