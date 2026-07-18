package pluginhost

import (
	"context"
	"fmt"
	"strings"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
)

// MigrationInput is passed through to the verified package host. Checkpoint
// and migration identifier values are package-owned opaque strings.
type MigrationInput struct {
	PackageID   string
	Version     string
	MigrationID string
	Checkpoint  string
	Generation  uint64
}

// MigrationOutput is the opaque reply returned by a package host. The kernel
// only persists and fences these values; it does not interpret their domain
// meaning.
type MigrationOutput struct {
	Checkpoint       string
	ValidationDigest string
	Complete         bool
	FailureCode      string
	HealthLeaseID    string
	HealthGeneration uint64
}

// MigrationCheckpointRecorder must durably persist each host reply before the
// migration caller can observe it. A nil recorder is rejected fail-closed.
type MigrationCheckpointRecorder func(context.Context, MigrationOutput) error

var _ MigrationManager = (*Supervisor)(nil)

func (c *hostClient) Migrate(ctx context.Context, input MigrationInput) (MigrationOutput, error) {
	if c == nil || c.rpc == nil {
		return MigrationOutput{}, ErrHostUnavailable
	}
	response, err := c.rpc.Migrate(ctx, &pluginhostv1.MigrationRequest{
		PackageId:       input.PackageID,
		PackageVersion:  input.Version,
		MigrationId:     input.MigrationID,
		Checkpoint:      input.Checkpoint,
		RouteGeneration: input.Generation,
	})
	if err != nil {
		return MigrationOutput{}, hostClientError(err)
	}
	return MigrationOutput{
		Checkpoint:       response.GetCheckpoint(),
		ValidationDigest: response.GetValidationDigest(),
		Complete:         response.GetComplete(),
		FailureCode:      response.GetFailureCode(),
	}, nil
}

// Migrate forwards one resumable migration step to the active host, persists
// its reply through recorder, and treats a completed response as activation
// only after the host health lease still matches the requested generation.
func (m *Supervisor) Migrate(ctx context.Context, input MigrationInput, recorder MigrationCheckpointRecorder) (MigrationOutput, error) {
	if m == nil {
		return MigrationOutput{}, ErrHostUnavailable
	}
	input.PackageID = strings.TrimSpace(input.PackageID)
	input.Version = strings.TrimSpace(input.Version)
	input.MigrationID = strings.TrimSpace(input.MigrationID)
	if input.PackageID == "" || input.Version == "" || input.MigrationID == "" || input.Generation == 0 {
		return MigrationOutput{}, ErrGenerationUnavailable
	}
	if recorder == nil {
		return MigrationOutput{}, fmt.Errorf("%w: migration checkpoint recorder is required", ErrHostIncompatible)
	}
	host, err := m.hostForGeneration(input.PackageID, input.Version, input.Generation)
	if err != nil {
		return MigrationOutput{}, err
	}
	response, err := host.client.Migrate(ctx, input)
	if err != nil {
		return MigrationOutput{}, err
	}
	if err := recorder(ctx, response); err != nil {
		return response, fmt.Errorf("%w: persist migration checkpoint: %v", ErrHostUnavailable, err)
	}
	if strings.TrimSpace(response.FailureCode) != "" {
		return response, fmt.Errorf("%w: migration failed with %q", ErrHostIncompatible, response.FailureCode)
	}
	if !response.Complete {
		return response, nil
	}
	if strings.TrimSpace(response.ValidationDigest) == "" {
		return response, fmt.Errorf("%w: completed migration omitted validation digest", ErrHostIncompatible)
	}
	health, err := m.Health(ctx, input.PackageID, input.Version, input.Generation)
	if err != nil {
		return response, err
	}
	if !health.Healthy || health.LeaseID == "" {
		return response, fmt.Errorf("%w: host is not healthy for route activation", ErrHostUnavailable)
	}
	response.HealthLeaseID = health.LeaseID
	response.HealthGeneration = input.Generation
	return response, nil
}
