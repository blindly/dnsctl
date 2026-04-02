package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/wk/dnsctl/internal/zone"
)

type RecordState struct {
	ID       string `json:"id"`
	TTL      int    `json:"ttl"`
	Proxied  *bool  `json:"proxied,omitempty"`
	Priority *int   `json:"priority,omitempty"`
}

type ZoneState struct {
	ZoneID  string                  `json:"zone_id"`
	Records map[string]*RecordState `json:"records"` // record key -> record state
}

type State struct {
	Zones map[string]*ZoneState `json:"zones,omitempty"`
}

type CreateOp struct {
	Record zone.Record
}

type UpdateOp struct {
	RecordID string
	Record   zone.Record
}

type DeleteOp struct {
	RecordID  string
	RecordKey string
}

type Changeset struct {
	Create []CreateOp
	Update []UpdateOp
	Delete []DeleteOp
}

// oldZoneState is the pre-v0.3.1 format where records mapped key -> ID string
type oldZoneState struct {
	ZoneID  string            `json:"zone_id"`
	Records map[string]string `json:"records"`
}

type oldState struct {
	Zones map[string]*oldZoneState `json:"zones,omitempty"`
}

func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Zones: make(map[string]*ZoneState)}, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	// Try new format first
	var s State
	if err := json.Unmarshal(data, &s); err == nil && s.Zones != nil {
		return &s, nil
	}

	// Fall back to old format and migrate
	var old oldState
	if err := json.Unmarshal(data, &old); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}

	migrated := &State{Zones: make(map[string]*ZoneState)}
	for name, ozs := range old.Zones {
		zs := &ZoneState{
			ZoneID:  ozs.ZoneID,
			Records: make(map[string]*RecordState),
		}
		for key, id := range ozs.Records {
			zs.Records[key] = &RecordState{ID: id}
		}
		migrated.Zones[name] = zs
	}

	// Save migrated state
	if err := Save(path, migrated); err != nil {
		return nil, fmt.Errorf("migrating state file: %w", err)
	}

	return migrated, nil
}

func Save(path string, s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func ComputeChangeset(zs *ZoneState, localRecords []zone.Record) Changeset {
	var cs Changeset
	seen := make(map[string]bool)

	for _, rec := range localRecords {
		key := rec.Key()
		seen[key] = true

		if rs, exists := zs.Records[key]; exists {
			// Only emit update if metadata actually changed
			if metadataChanged(rs, rec) {
				cs.Update = append(cs.Update, UpdateOp{RecordID: rs.ID, Record: rec})
			}
		} else {
			cs.Create = append(cs.Create, CreateOp{Record: rec})
		}
	}

	for key, rs := range zs.Records {
		if !seen[key] {
			cs.Delete = append(cs.Delete, DeleteOp{RecordID: rs.ID, RecordKey: key})
		}
	}

	return cs
}

func metadataChanged(rs *RecordState, rec zone.Record) bool {
	if rs.TTL != rec.TTL {
		return true
	}
	if boolChanged(rs.Proxied, rec.Proxied) {
		return true
	}
	if intChanged(rs.Priority, rec.Priority) {
		return true
	}
	return false
}

func boolChanged(a, b *bool) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil || b == nil {
		return true
	}
	return *a != *b
}

func intChanged(a, b *int) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil || b == nil {
		return true
	}
	return *a != *b
}
