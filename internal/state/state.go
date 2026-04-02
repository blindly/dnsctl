package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/wk/dnsctl/internal/zone"
)

type ZoneState struct {
	ZoneID  string            `json:"zone_id"`
	Records map[string]string `json:"records"`
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

func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Zones: make(map[string]*ZoneState)}, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	if s.Zones == nil {
		s.Zones = make(map[string]*ZoneState)
	}
	return &s, nil
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

		if recordID, exists := zs.Records[key]; exists {
			cs.Update = append(cs.Update, UpdateOp{RecordID: recordID, Record: rec})
		} else {
			cs.Create = append(cs.Create, CreateOp{Record: rec})
		}
	}

	for key, recordID := range zs.Records {
		if !seen[key] {
			cs.Delete = append(cs.Delete, DeleteOp{RecordID: recordID, RecordKey: key})
		}
	}

	return cs
}
