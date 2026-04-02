package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSaveConfig(t *testing.T) {
	dir := t.TempDir()
	dnsctlDir := filepath.Join(dir, ".dnsctl")
	err := os.MkdirAll(dnsctlDir, 0755)
	require.NoError(t, err)

	cfg := &Config{
		Provider: "cloudflare",
		Zones:    []string{"example.com", "test.dev"},
	}

	path := filepath.Join(dnsctlDir, "config.yaml")
	err = Save(path, cfg)
	require.NoError(t, err)

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "cloudflare", loaded.Provider)
	assert.Equal(t, []string{"example.com", "test.dev"}, loaded.Zones)
}

func TestFindRoot(t *testing.T) {
	dir := t.TempDir()
	dnsctlDir := filepath.Join(dir, ".dnsctl")
	err := os.MkdirAll(dnsctlDir, 0755)
	require.NoError(t, err)

	root, err := FindRoot(dir)
	require.NoError(t, err)
	assert.Equal(t, dir, root)
}

func TestFindRootNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := FindRoot(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a dnsctl repository")
}
