package main

import (
	"fmt"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// buildSystemConfigs converts old v2_settings rows into model.SystemConfig.
// The old schema has no type discriminator, so every migrated value is
// stored with Type "string"; admins can adjust individual entries in the
// admin panel afterward if a more specific type is needed.
func buildSystemConfigs(t *DumpTable) ([]*model.SystemConfig, error) {
	configs := make([]*model.SystemConfig, 0, len(t.Rows))
	for _, r := range t.Rows {
		id, err := mustUint(r, "id")
		if err != nil {
			return nil, fmt.Errorf("v2_settings: %w", err)
		}

		configs = append(configs, &model.SystemConfig{
			ID:    id,
			Key:   str(r, "name"),
			Value: str(r, "value"),
			Type:  "string",
			Group: str(r, "group"),
		})
	}
	return configs, nil
}
