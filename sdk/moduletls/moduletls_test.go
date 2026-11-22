package moduletls

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKernelIdentity(t *testing.T) {
	kernel, err := Kernel("prod-eu")
	require.NoError(t, err)
	require.Equal(t, "spiffe://anixops/prod-eu/kernel", kernel.String())
	require.Equal(t, kernel.String(), kernel.URL().String())
	for _, cluster := range []string{"", "Prod", "-prod", "prod_eu"} {
		_, err := Kernel(cluster)
		require.ErrorIs(t, err, ErrInvalidIdentity, cluster)
	}
}
