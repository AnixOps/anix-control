package grpc

import (
	"fmt"
	"math"

	pb "github.com/AnixOps/anix-control/v3/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v3/internal/model"
)

const (
	maxInt32Value  = int64(1<<31 - 1)
	minInt32Value  = int64(-1 << 31)
	maxUint32Value = uint64(1<<32 - 1)
)

func uintToUint32(label string, v uint) (uint32, error) {
	if uint64(v) > maxUint32Value {
		return 0, fmt.Errorf("%s %d exceeds uint32 range", label, v)
	}
	return uint32(v), nil
}

func intToInt32(label string, v int) (int32, error) {
	return int64ToInt32(label, int64(v))
}

func int64ToInt32(label string, v int64) (int32, error) {
	if v < minInt32Value || v > maxInt32Value {
		return 0, fmt.Errorf("%s %d exceeds int32 range", label, v)
	}
	return int32(v), nil
}

func anyToInt32(label string, v any) (int32, bool, error) {
	switch t := v.(type) {
	case int:
		n, err := intToInt32(label, t)
		return n, err == nil, err
	case int32:
		return t, true, nil
	case int64:
		n, err := int64ToInt32(label, t)
		return n, err == nil, err
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) || t < float64(minInt32Value) || t > float64(maxInt32Value) {
			return 0, false, fmt.Errorf("%s %v exceeds int32 range", label, t)
		}
		return int32(t), true, nil
	default:
		return 0, false, nil
	}
}

func userInfoFromModel(user *model.User) (*pb.UserInfo, error) {
	if user == nil {
		return nil, fmt.Errorf("user is nil")
	}

	userID, err := uintToUint32("user id", user.ID)
	if err != nil {
		return nil, err
	}

	deviceLimit, err := intToInt32("device limit", user.GetDeviceLimit())
	if err != nil {
		return nil, err
	}

	return &pb.UserInfo{
		Id:             userID,
		Uuid:           user.UUID,
		SpeedLimit:     user.GetSpeedLimit(),
		DeviceLimit:    deviceLimit,
		TransferEnable: user.TransferEnable,
		UsedUpload:     user.U,
		UsedDownload:   user.D,
	}, nil
}
