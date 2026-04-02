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

	proxied := true
	s := &State{
		Zones: map[string]*ZoneState{
			"example.com": {
				ZoneID: "zone-123",
				Records: map[string]*RecordState{
					"a]@]192.0.2.1": {ID: "rec-abc", TTL: 300, Proxied: &proxied},
				},
			},
		},
	}

	err := Save(path, s)
	require.NoError(t, err)

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "zone-123", loaded.Zones["example.com"].ZoneID)
	assert.Equal(t, "rec-abc", loaded.Zones["example.com"].Records["a]@]192.0.2.1"].ID)
	assert.Equal(t, 300, loaded.Zones["example.com"].Records["a]@]192.0.2.1"].TTL)
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	s, err := Load("/nonexistent/state.json")
	require.NoError(t, err)
	assert.NotNil(t, s.Zones)
	assert.Empty(t, s.Zones)
}

func TestComputeChangeset(t *testing.T) {
	proxied := true
	s := &ZoneState{
		ZoneID: "zone-123",
		Records: map[string]*RecordState{
			"a]@]192.0.2.1":        {ID: "rec-1", TTL: 300, Proxied: &proxied},
			"a]www]192.0.2.1":      {ID: "rec-2", TTL: 300, Proxied: &proxied},
			"cname]old]example.com": {ID: "rec-3", TTL: 300},
		},
	}

	localRecords := []zone.Record{
		{Type: "A", Name: "@", Content: "192.0.2.1", TTL: 300, Proxied: &proxied},        // unchanged
		{Type: "A", Name: "www", Content: "10.0.0.1", TTL: 300, Proxied: &proxied},       // changed content (new key)
		{Type: "CNAME", Name: "new", Content: "new.example.com", TTL: 300},                // new record
		// "old" CNAME removed — should be a delete
	}

	cs := ComputeChangeset(s, localRecords)

	assert.Len(t, cs.Create, 2)  // new www IP + new CNAME
	assert.Len(t, cs.Update, 0)  // A @ record unchanged — no update
	assert.Len(t, cs.Delete, 2)  // old www IP + old CNAME
}

func TestComputeChangesetDetectsMetadataChange(t *testing.T) {
	proxied := true
	notProxied := false
	s := &ZoneState{
		ZoneID: "zone-123",
		Records: map[string]*RecordState{
			"a]@]192.0.2.1": {ID: "rec-1", TTL: 300, Proxied: &proxied},
		},
	}

	localRecords := []zone.Record{
		{Type: "A", Name: "@", Content: "192.0.2.1", TTL: 600, Proxied: &notProxied}, // TTL and proxied changed
	}

	cs := ComputeChangeset(s, localRecords)

	assert.Len(t, cs.Create, 0)
	assert.Len(t, cs.Update, 1)  // metadata changed
	assert.Len(t, cs.Delete, 0)
	assert.Equal(t, "rec-1", cs.Update[0].RecordID)
}
