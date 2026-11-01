package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIpsToInterface(t *testing.T) {
	ips := []string{"192.168.1.1", "10.0.0.1"}
	result := ipsToInterface(ips)

	assert.Len(t, result, 2)
	assert.Equal(t, "192.168.1.1", result[0])
	assert.Equal(t, "10.0.0.1", result[1])
}
