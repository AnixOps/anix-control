package grpc

import "fmt"

const (
	maxInt32Value = int64(1<<31 - 1)
	minInt32Value = int64(-1 << 31)
)

func intToInt32(label string, v int) (int32, error) {
	return int64ToInt32(label, int64(v))
}

func int64ToInt32(label string, v int64) (int32, error) {
	if v < minInt32Value || v > maxInt32Value {
		return 0, fmt.Errorf("%s %d exceeds int32 range", label, v)
	}
	return int32(v), nil
}
