package meshregion

import (
	"strconv"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The kernel capabilities that add the region object and the region
// request state to a configuration (appendix D.9.5; spec mesh-status.md).
// An Agent leaves out what its kernel does not list, so the panel always
// sends them to an Agent node; a device's configuration follows the
// capabilities the device names (DeviceConfig).
const (
	FeatureRegionProbe        = "mesh_region_probe"
	FeatureRegionRequestState = "mesh_region_request_state"
)

// basisValue is the basis stored on a row: admin when it has none.
func basisValue(stored *string) string {
	if stored == nil || *stored == "" {
		return model.MeshBasisAdmin
	}
	return *stored
}

// regionObject is the top-level "region" of a configuration (appendix
// D.9.1): current is the code of the region the configuration delivers a
// network of, nil (with a nil basis) when it delivers none.
func regionObject(directory Directory, current *model.MeshRegion, basis string, responder map[string]any) map[string]any {
	object := map[string]any{"current": nil, "basis": nil, "directory": directory, "responder": nil}
	if current != nil {
		object["current"], object["basis"] = current.Code, basis
	}
	if responder != nil {
		object["responder"] = responder
	}
	return object
}

// responderSection is the probe responder a border node is told to run
// (appendix D.9.3), on the responder port of the settings.
func responderSection(db *gorm.DB) (map[string]any, error) {
	settings, err := loadSettings(db)
	if err != nil {
		return nil, err
	}
	port := strconv.Itoa(settings.ResponderPort)
	return map[string]any{"listen": []string{"0.0.0.0:" + port, "[::]:" + port}}, nil
}

// defaultDirectory is the directory of a configuration: the preferred
// region is the settings' default.
func defaultDirectory(db *gorm.DB) (Directory, error) {
	settings, err := loadSettings(db)
	if err != nil {
		return Directory{}, err
	}
	return BuildDirectory(db, settings.PreferredRegionID)
}

// regionRequestValue is the "region_request" of a network (appendix D.7
// item 3): null, or the member's latest region change while it is pending,
// or rejected and not since superseded (a newer change of the member,
// withdrawn or approved, replaces it). The member is a node (nodeRef) or a
// device (deviceID).
func regionRequestValue(db *gorm.DB, nodeRef string, deviceID uint) (any, error) {
	query := db.Order("id DESC").Limit(1)
	if nodeRef != "" {
		query = query.Where("node_ref = ?", nodeRef)
	} else {
		query = query.Where("device_id = ?", deviceID)
	}
	var rows []model.MeshRegionChange
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 || (rows[0].Status != model.MeshChangePending && rows[0].Status != model.MeshChangeRejected) {
		return nil, nil
	}
	codes, err := regionCodes(db)
	if err != nil {
		return nil, err
	}
	row := rows[0]
	value := map[string]any{"id": strconv.FormatUint(uint64(row.ID), 10), "region": codes[row.ToRegionID], "state": row.Status, "reason": row.Reason, "reject_reason": ""}
	if row.Status == model.MeshChangeRejected {
		value["reject_reason"] = row.Note
	}
	return value, nil
}
