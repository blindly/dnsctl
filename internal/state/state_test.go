package state

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wk/dnsctl/internal/zone"
)

func TestLoadSaveState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{
		Zones: map[string]*ZoneState{
			"example.com": {
				ZoneID:  "zone-123",
				Records: map[string]string{
					"a]@]192.0.2.1": "rec-abc",
				},
			},
		},
	}

	err := Save(path, s)
	require.NoError(t, err)

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "zone-123", loaded.Zones["example.com"].ZoneID)
	assert.Equal(t, "rec-abc", loaded.Zones["example.com"].Records["a]@]192.0.2.1"])
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	s, err := Load("/nonexistent/state.json")
	require.NoError(t, err)
	assert.NotNil(t, s.Zones)
	assert.Empty(t, s.Zones)
}

func TestComputeChangeset(t *testing.T) {
	s := &ZoneState{
		ZoneID: "zone-123",
		Records: map[string]string{
			"a]@]192.0.2.1":        "rec-1",
			"a]www]192.0.2.1":      "rec-2",
			"cname]old]example.com": "rec-3",
		},
	}

	proxied := true
	localRecords := []zone.Record{
		{Type: "A", Name: "@", Content: "192.0.2.1", TTL: 300, Proxied: &proxied},
		{Type: "A", Name: "www", Content: "10.0.0.1", TTL: 300, Proxied: &proxied},  // changed content
		{Type: "CNAME", Name: "new", Content: "new.example.com", TTL: 300},           // new record
		// "old" CNAME removed — should be a delete
	}

	cs := ComputeChangeset(s, localRecords)

	// "a]@]192.0.2.1" exists in state — it's an update
	// "a]www]10.0.0.1" does NOT exist in state (content changed) — it's a create
	// "cname]new]new.example.com" does NOT exist in state — it's a create
	// "a]www]192.0.2.1" is in state but NOT in local — it's a delete
	// "cname]old]example.com" is in state but NOT in local — it's a delete

	assert.Len(t, cs.Create, 2) // new www IP + new CNAME
	assert.Len(t, cs.Update, 1) // existing A @ record
	assert.Len(t, cs.Delete, 2) // old www IP + old CNAME
}
