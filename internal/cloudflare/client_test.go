package cloudflare

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientMissingToken(t *testing.T) {
	_, err := NewClient("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLOUDFLARE_API_TOKEN")
}

func TestNewClientValidToken(t *testing.T) {
	client, err := NewClient("test-token-123")
	require.NoError(t, err)
	assert.NotNil(t, client)
}
