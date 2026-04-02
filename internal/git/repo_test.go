package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitAndCommit(t *testing.T) {
	dir := t.TempDir()

	repo, err := Init(dir)
	require.NoError(t, err)

	// Write a file
	err = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
	require.NoError(t, err)

	err = repo.AddAndCommit("initial commit", "test.txt")
	require.NoError(t, err)

	log, err := repo.Log(10)
	require.NoError(t, err)
	assert.Len(t, log, 1)
	assert.Contains(t, log[0].Message, "initial commit")
}

func TestOpenExisting(t *testing.T) {
	dir := t.TempDir()

	_, err := Init(dir)
	require.NoError(t, err)

	repo, err := Open(dir)
	require.NoError(t, err)
	assert.NotNil(t, repo)
}

func TestStatusClean(t *testing.T) {
	dir := t.TempDir()

	repo, err := Init(dir)
	require.NoError(t, err)

	status, err := repo.Status()
	require.NoError(t, err)
	assert.True(t, status.IsClean())
}

func TestStatusDirty(t *testing.T) {
	dir := t.TempDir()

	repo, err := Init(dir)
	require.NoError(t, err)

	// Write and commit a file
	err = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
	require.NoError(t, err)
	err = repo.AddAndCommit("initial", "test.txt")
	require.NoError(t, err)

	// Modify the file
	err = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("changed"), 0644)
	require.NoError(t, err)

	status, err := repo.Status()
	require.NoError(t, err)
	assert.False(t, status.IsClean())
}
