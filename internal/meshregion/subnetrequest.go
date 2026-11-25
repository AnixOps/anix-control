package meshregion

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Subnet requests (R35): a full node without an Agent, such as an OpenWrt
// router, asks with its device token to advertise subnets. The request
// waits for an administrator, who approves it (it becomes the node's
// confirmed subnets, checked like SetDeviceSubnets) or rejects it (the
// confirmed subnets stay). A node has at most one request pending; a new
// one replaces it, and an empty one withdraws it.

// maxRequestedSubnets bounds the subnets of one request.
const maxRequestedSubnets = 16

// SubnetRequest is a full node's pending request.
type SubnetRequest struct {
	Subnets     []string  `json:"subnets"`
	RequestedAt time.Time `json:"requested_at"`
}

// pendingRequest answers the node's pending request, or nil.
func pendingRequest(row model.MeshFullNode) *SubnetRequest {
	if row.RequestedSubnets == nil || row.RequestedAt == nil {
		return nil
	}
	return &SubnetRequest{Subnets: splitSubnets(*row.RequestedSubnets), RequestedAt: *row.RequestedAt}
}

// RequestDeviceSubnets records the subnets device asks to advertise:
// private IPv4 networks with no host bits, at most 16; none withdraws the
// pending request. Only a full node asks (ErrConflict otherwise). Overlaps
// are checked when an administrator approves.
func RequestDeviceSubnets(ctx context.Context, db *gorm.DB, device model.MeshDevice, raw []string, now time.Time) error {
	subnets, err := parseSubnets("cidrs", raw, maxRequestedSubnets)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, full, err := isFullNode(tx, device.ID)
		if err != nil {
			return err
		}
		if !full {
			return conflict("only a full node advertises subnets")
		}
		update := map[string]any{"requested_subnets": nil, "requested_at": nil, "updated_at": now}
		if len(subnets) > 0 {
			update["requested_subnets"], update["requested_at"] = strings.Join(subnets, ","), now
		}
		return tx.Model(&row).Updates(update).Error
	})
}

// pendingOf answers the full node of deviceID and its pending request,
// ErrNotFound when there is none.
func pendingOf(tx *gorm.DB, deviceID uint) (model.MeshFullNode, SubnetRequest, error) {
	row, full, err := isFullNode(tx, deviceID)
	if err != nil {
		return row, SubnetRequest{}, err
	}
	if !full {
		var device model.MeshDevice
		if err := findDevice(tx, deviceID, &device); err != nil {
			return row, SubnetRequest{}, err
		}
	}
	request := pendingRequest(row)
	if !full || request == nil {
		return row, SubnetRequest{}, fmt.Errorf("%w: subnet request of device %d", ErrNotFound, deviceID)
	}
	return row, *request, nil
}

// clearRequest removes the request of row if it is still the one read.
func clearRequest(tx *gorm.DB, row model.MeshFullNode, request SubnetRequest, now time.Time) error {
	result := tx.Model(&model.MeshFullNode{}).Where("id = ? AND requested_subnets = ?", row.ID, strings.Join(request.Subnets, ",")).
		Updates(map[string]any{"requested_subnets": nil, "requested_at": nil, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return conflict("the device changed its subnet request")
	}
	return nil
}

// ApproveDeviceSubnets confirms the pending request of the full node
// deviceID, which must still list subnets (what the administrator saw;
// ErrConflict otherwise), as its subnets, with the checks of
// SetDeviceSubnets. It answers the confirmed subnets.
func ApproveDeviceSubnets(ctx context.Context, db *gorm.DB, deviceID uint, subnets []string, now time.Time) ([]string, error) {
	seen, err := parseSubnets("subnets", subnets, maxRequestedSubnets)
	if err != nil {
		return nil, err
	}
	var approved []string
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAddressPlan(tx); err != nil {
			return err
		}
		row, request, err := pendingOf(tx, deviceID)
		if err != nil {
			return err
		}
		if !slices.Equal(request.Subnets, seen) {
			return conflict("the device changed its subnet request")
		}
		if err := setSubnets(tx, deviceID, request.Subnets, now); err != nil {
			return err
		}
		approved = request.Subnets
		return clearRequest(tx, row, request, now)
	})
	return approved, err
}

// RejectDeviceSubnets removes the pending request of the full node
// deviceID; its confirmed subnets stay. It answers the rejected subnets.
func RejectDeviceSubnets(ctx context.Context, db *gorm.DB, deviceID uint, now time.Time) ([]string, error) {
	var rejected []string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, request, err := pendingOf(tx, deviceID)
		if err != nil {
			return err
		}
		rejected = request.Subnets
		return clearRequest(tx, row, request, now)
	})
	return rejected, err
}
