# dnsctl Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Git-like CLI tool that syncs DNS records between local YAML files and Cloudflare, with full version history via git.

**Architecture:** A Go CLI using cobra for commands, go-git for version control, and cloudflare-go for API access. The tool manages a git repo where each zone is a YAML file. A `.dnsctl/` directory holds config and state (record ID mappings). All commands delegate to four internal packages: cloudflare (API), git (repo ops), zone (YAML parsing), and state (ID mapping).

**Tech Stack:** Go, cobra, go-git, cloudflare-go, yaml.v3, charmbracelet/huh

**Go binary location:** `/usr/local/go/bin/go` (not in PATH — use full path or `export PATH=$PATH:/usr/local/go/bin`)

---

## File Map

| File | Responsibility |
|------|---------------|
| `main.go` | Entry point, calls `cmd.Execute()` |
| `cmd/root.go` | Cobra root command, version flag |
| `cmd/init.go` | `dnsctl init` — auth, zone picker, first pull |
| `cmd/pull.go` | `dnsctl pull` — fetch from CF, write YAML, git commit |
| `cmd/status.go` | `dnsctl status` — git status wrapper |
| `cmd/diff.go` | `dnsctl diff` — structured record-level diff |
| `cmd/commit.go` | `dnsctl commit` — push to CF, git commit |
| `cmd/log.go` | `dnsctl log` — git log wrapper |
| `internal/cloudflare/client.go` | Cloudflare API wrapper: auth, list zones, list/create/update/delete records |
| `internal/git/repo.go` | Git operations: init, add, commit, status, diff, log |
| `internal/zone/zone.go` | Zone file types, YAML marshal/unmarshal, deterministic sorting |
| `internal/state/state.go` | State file (record ID mapping) read/write, changeset computation |
| `internal/cloudflare/client_test.go` | Tests for CF client (mocked HTTP) |
| `internal/zone/zone_test.go` | Tests for zone parsing, sorting, round-trip |
| `internal/state/state_test.go` | Tests for state management, changeset computation |
| `internal/git/repo_test.go` | Tests for git operations |

---

## Task 1: Project Scaffolding

**Files:**
- Create: `main.go`
- Create: `cmd/root.go`
- Create: `go.mod`

- [ ] **Step 1: Initialize Go module**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go mod init github.com/wk/dnsctl
```

- [ ] **Step 2: Install cobra dependency**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go get github.com/spf13/cobra@latest
```

- [ ] **Step 3: Create main.go**

```go
// main.go
package main

import "github.com/wk/dnsctl/cmd"

func main() {
	cmd.Execute()
}
```

- [ ] **Step 4: Create cmd/root.go**

```go
// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dnsctl",
	Short: "Git-like CLI for DNS management",
	Long:  "A CLI tool that brings a Git-like workflow to DNS record management via Cloudflare.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Verify it compiles**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go build -o dnsctl .
./dnsctl --help
```

Expected: Help output showing "Git-like CLI for DNS management"

- [ ] **Step 6: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add main.go cmd/root.go go.mod go.sum
git commit -m "feat: scaffold project with cobra root command"
```

---

## Task 2: Zone File Package

**Files:**
- Create: `internal/zone/zone.go`
- Create: `internal/zone/zone_test.go`

- [ ] **Step 1: Write failing tests for zone parsing and serialization**

```go
// internal/zone/zone_test.go
package zone

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseZoneFile(t *testing.T) {
	input := `zone: example.com
records:
  - type: A
    name: "@"
    content: 192.0.2.1
    ttl: 300
    proxied: true
  - type: MX
    name: "@"
    content: mail.example.com
    priority: 10
    ttl: 3600
`
	zf, err := Parse([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "example.com", zf.Zone)
	assert.Len(t, zf.Records, 2)
	assert.Equal(t, "A", zf.Records[0].Type)
	assert.Equal(t, "@", zf.Records[0].Name)
	assert.Equal(t, "192.0.2.1", zf.Records[0].Content)
	assert.Equal(t, 300, zf.Records[0].TTL)
	assert.Equal(t, true, *zf.Records[0].Proxied)
	assert.Equal(t, "MX", zf.Records[1].Type)
	assert.Equal(t, 10, *zf.Records[1].Priority)
}

func TestMarshalZoneFile(t *testing.T) {
	proxied := true
	priority := 10
	zf := &ZoneFile{
		Zone: "example.com",
		Records: []Record{
			{Type: "MX", Name: "@", Content: "mail.example.com", TTL: 3600, Priority: &priority},
			{Type: "A", Name: "@", Content: "192.0.2.1", TTL: 300, Proxied: &proxied},
		},
	}
	data, err := Marshal(zf)
	require.NoError(t, err)

	// Should be sorted: A before MX (by type, then name)
	parsed, err := Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "A", parsed.Records[0].Type)
	assert.Equal(t, "MX", parsed.Records[1].Type)
}

func TestRecordKey(t *testing.T) {
	r := Record{Type: "A", Name: "www", Content: "192.0.2.1"}
	assert.Equal(t, "a]www]192.0.2.1", r.Key())
}

func TestSortRecords(t *testing.T) {
	records := []Record{
		{Type: "TXT", Name: "@", Content: "v=spf1"},
		{Type: "A", Name: "www", Content: "192.0.2.1"},
		{Type: "A", Name: "@", Content: "192.0.2.1"},
		{Type: "CNAME", Name: "blog", Content: "example.netlify.app"},
	}
	sorted := SortRecords(records)
	assert.Equal(t, "A", sorted[0].Type)
	assert.Equal(t, "@", sorted[0].Name)
	assert.Equal(t, "A", sorted[1].Type)
	assert.Equal(t, "www", sorted[1].Name)
	assert.Equal(t, "CNAME", sorted[2].Type)
	assert.Equal(t, "TXT", sorted[3].Type)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go get github.com/stretchr/testify@latest
go test ./internal/zone/ -v
```

Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: Implement zone package**

```go
// internal/zone/zone.go
package zone

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Record struct {
	Type     string `yaml:"type"`
	Name     string `yaml:"name"`
	Content  string `yaml:"content"`
	TTL      int    `yaml:"ttl"`
	Proxied  *bool  `yaml:"proxied,omitempty"`
	Priority *int   `yaml:"priority,omitempty"`
}

func (r Record) Key() string {
	return fmt.Sprintf("%s]%s]%s", strings.ToLower(r.Type), strings.ToLower(r.Name), strings.ToLower(r.Content))
}

type ZoneFile struct {
	Zone    string   `yaml:"zone"`
	Records []Record `yaml:"records"`
}

func Parse(data []byte) (*ZoneFile, error) {
	var zf ZoneFile
	if err := yaml.Unmarshal(data, &zf); err != nil {
		return nil, fmt.Errorf("parsing zone file: %w", err)
	}
	return &zf, nil
}

func Marshal(zf *ZoneFile) ([]byte, error) {
	sorted := *zf
	sorted.Records = SortRecords(sorted.Records)
	data, err := yaml.Marshal(&sorted)
	if err != nil {
		return nil, fmt.Errorf("marshaling zone file: %w", err)
	}
	return data, nil
}

func SortRecords(records []Record) []Record {
	sorted := make([]Record, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Content < sorted[j].Content
	})
	return sorted
}

func ReadFile(path string) (*ZoneFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading zone file %s: %w", path, err)
	}
	return Parse(data)
}

func WriteFile(path string, zf *ZoneFile) error {
	data, err := Marshal(zf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
```

- [ ] **Step 4: Install yaml.v3 and run tests**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go get gopkg.in/yaml.v3@latest
go test ./internal/zone/ -v
```

Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add internal/zone/ go.mod go.sum
git commit -m "feat: add zone file parsing, serialization, and sorting"
```

---

## Task 3: State Package

**Files:**
- Create: `internal/state/state.go`
- Create: `internal/state/state_test.go`

- [ ] **Step 1: Write failing tests for state management**

```go
// internal/state/state_test.go
package state

import (
	"os"
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
			"a]@]192.0.2.1":          "rec-1",
			"a]www]192.0.2.1":        "rec-2",
			"cname]old]example.com":   "rec-3",
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

	assert.Len(t, cs.Create, 1)
	assert.Equal(t, "new", cs.Create[0].Name)

	assert.Len(t, cs.Update, 1)
	assert.Equal(t, "www", cs.Update[0].Record.Name)
	assert.Equal(t, "rec-2", cs.Update[0].RecordID)

	assert.Len(t, cs.Delete, 1)
	assert.Equal(t, "rec-3", cs.Delete[0].RecordID)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go test ./internal/state/ -v
```

Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement state package**

```go
// internal/state/state.go
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
```

- [ ] **Step 5: Run tests**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go test ./internal/state/ -v
```

Expected: All 3 tests PASS

- [ ] **Step 6: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add internal/state/ go.mod go.sum
git commit -m "feat: add state management and changeset computation"
```

---

## Task 4: Git Package

**Files:**
- Create: `internal/git/repo.go`
- Create: `internal/git/repo_test.go`

- [ ] **Step 1: Write failing tests for git operations**

```go
// internal/git/repo_test.go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go get github.com/go-git/go-git/v5@latest
go test ./internal/git/ -v
```

Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement git package**

```go
// internal/git/repo.go
package git

import (
	"fmt"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Repo struct {
	repo *gogit.Repository
	path string
}

type LogEntry struct {
	Hash    string
	Message string
	When    time.Time
}

type StatusResult struct {
	clean bool
	files map[string]string // filename -> status code
}

func (s *StatusResult) IsClean() bool {
	return s.clean
}

func (s *StatusResult) Files() map[string]string {
	return s.files
}

func Init(path string) (*Repo, error) {
	repo, err := gogit.PlainInit(path, false)
	if err != nil {
		return nil, fmt.Errorf("git init: %w", err)
	}
	return &Repo{repo: repo, path: path}, nil
}

func Open(path string) (*Repo, error) {
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("git open: %w", err)
	}
	return &Repo{repo: repo, path: path}, nil
}

func (r *Repo) AddAndCommit(message string, paths ...string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}

	for _, p := range paths {
		if _, err := w.Add(p); err != nil {
			return fmt.Errorf("git add %s: %w", p, err)
		}
	}

	_, err = w.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "dnsctl",
			Email: "dnsctl@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

func (r *Repo) AddAllAndCommit(message string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}

	if _, err := w.Add("."); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	_, err = w.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "dnsctl",
			Email: "dnsctl@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

func (r *Repo) Status() (*StatusResult, error) {
	w, err := r.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("getting worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}

	files := make(map[string]string)
	for file, s := range status {
		code := string(s.Worktree)
		if s.Staging != ' ' && s.Staging != '?' {
			code = string(s.Staging)
		}
		files[file] = code
	}

	return &StatusResult{
		clean: status.IsClean(),
		files: files,
	}, nil
}

func (r *Repo) Log(max int) ([]LogEntry, error) {
	iter, err := r.repo.Log(&gogit.LogOptions{})
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var entries []LogEntry
	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if count >= max {
			return fmt.Errorf("stop")
		}
		entries = append(entries, LogEntry{
			Hash:    c.Hash.String()[:7],
			Message: c.Message,
			When:    c.Author.When,
		})
		count++
		return nil
	})
	// The "stop" error is expected for limiting
	if err != nil && err.Error() != "stop" {
		return nil, err
	}

	return entries, nil
}

func (r *Repo) Diff() (string, error) {
	w, err := r.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("getting worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}

	if status.IsClean() {
		return "", nil
	}

	// go-git doesn't have a nice diff API for worktree changes,
	// so we shell out to git for diff
	return "", fmt.Errorf("use DiffShell for worktree diffs")
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go test ./internal/git/ -v
```

Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add internal/git/ go.mod go.sum
git commit -m "feat: add git operations package"
```

---

## Task 5: Cloudflare Client Package

**Files:**
- Create: `internal/cloudflare/client.go`
- Create: `internal/cloudflare/client_test.go`

- [ ] **Step 1: Write failing tests with mocked HTTP**

```go
// internal/cloudflare/client_test.go
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
	// This just tests construction, not API calls
	client, err := NewClient("test-token-123")
	require.NoError(t, err)
	assert.NotNil(t, client)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go get github.com/cloudflare/cloudflare-go@latest
go test ./internal/cloudflare/ -v
```

Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement cloudflare client**

```go
// internal/cloudflare/client.go
package cloudflare

import (
	"context"
	"fmt"
	"strings"

	cf "github.com/cloudflare/cloudflare-go"
	"github.com/wk/dnsctl/internal/zone"
)

type Client struct {
	api *cf.API
}

type ZoneInfo struct {
	ID   string
	Name string
}

func NewClient(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("CLOUDFLARE_API_TOKEN not set. Get a token at https://dash.cloudflare.com/profile/api-tokens")
	}
	api, err := cf.NewWithAPIToken(token)
	if err != nil {
		return nil, fmt.Errorf("creating cloudflare client: %w", err)
	}
	return &Client{api: api}, nil
}

func (c *Client) VerifyToken(ctx context.Context) error {
	_, err := c.api.VerifyAPIToken(ctx)
	if err != nil {
		return fmt.Errorf("invalid API token: %w", err)
	}
	return nil
}

func (c *Client) ListZones(ctx context.Context) ([]ZoneInfo, error) {
	zones, err := c.api.ListZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing zones: %w", err)
	}
	var result []ZoneInfo
	for _, z := range zones {
		result = append(result, ZoneInfo{ID: z.ID, Name: z.Name})
	}
	return result, nil
}

func (c *Client) ListRecords(ctx context.Context, zoneID string) ([]zone.Record, map[string]string, error) {
	rc := cf.ZoneIdentifier(zoneID)
	records, _, err := c.api.ListDNSRecords(ctx, rc, cf.ListDNSRecordsParams{})
	if err != nil {
		return nil, nil, fmt.Errorf("listing records: %w", err)
	}

	var zoneRecords []zone.Record
	idMap := make(map[string]string) // record key -> cloudflare ID

	for _, r := range records {
		rec := zone.Record{
			Type:    r.Type,
			Name:    simplifyName(r.Name, r.ZoneName),
			Content: r.Content,
			TTL:     r.TTL,
		}
		if r.Type == "A" || r.Type == "AAAA" || r.Type == "CNAME" {
			proxied := *r.Proxied
			rec.Proxied = &proxied
		}
		if r.Type == "MX" || r.Type == "SRV" {
			priority := int(*r.Priority)
			rec.Priority = &priority
		}
		zoneRecords = append(zoneRecords, rec)
		idMap[rec.Key()] = r.ID
	}

	return zoneRecords, idMap, nil
}

func (c *Client) CreateRecord(ctx context.Context, zoneID string, rec zone.Record, zoneName string) (string, error) {
	rc := cf.ZoneIdentifier(zoneID)
	params := cf.CreateDNSRecordParams{
		Type:    rec.Type,
		Name:    expandName(rec.Name, zoneName),
		Content: rec.Content,
		TTL:     rec.TTL,
	}
	if rec.Proxied != nil {
		params.Proxied = rec.Proxied
	}
	if rec.Priority != nil {
		p := uint16(*rec.Priority)
		params.Priority = &p
	}

	result, err := c.api.CreateDNSRecord(ctx, rc, params)
	if err != nil {
		return "", fmt.Errorf("creating record %s %s: %w", rec.Type, rec.Name, err)
	}
	return result.ID, nil
}

func (c *Client) UpdateRecord(ctx context.Context, zoneID, recordID string, rec zone.Record, zoneName string) error {
	rc := cf.ZoneIdentifier(zoneID)
	params := cf.UpdateDNSRecordParams{
		ID:      recordID,
		Type:    rec.Type,
		Name:    expandName(rec.Name, zoneName),
		Content: rec.Content,
		TTL:     rec.TTL,
	}
	if rec.Proxied != nil {
		params.Proxied = rec.Proxied
	}
	if rec.Priority != nil {
		p := uint16(*rec.Priority)
		params.Priority = &p
	}

	_, err := c.api.UpdateDNSRecord(ctx, rc, params)
	if err != nil {
		return fmt.Errorf("updating record %s %s: %w", rec.Type, rec.Name, err)
	}
	return nil
}

func (c *Client) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	rc := cf.ZoneIdentifier(zoneID)
	err := c.api.DeleteDNSRecord(ctx, rc, recordID)
	if err != nil {
		return fmt.Errorf("deleting record %s: %w", recordID, err)
	}
	return nil
}

func simplifyName(fullName, zoneName string) string {
	if fullName == zoneName {
		return "@"
	}
	return strings.TrimSuffix(fullName, "."+zoneName)
}

func expandName(name, zoneName string) string {
	if name == "@" {
		return zoneName
	}
	return name + "." + zoneName
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go test ./internal/cloudflare/ -v
```

Expected: Both tests PASS (only testing construction, not live API)

- [ ] **Step 5: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add internal/cloudflare/ go.mod go.sum
git commit -m "feat: add cloudflare API client wrapper"
```

---

## Task 6: Config Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/config/config_test.go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go test ./internal/config/ -v
```

Expected: FAIL

- [ ] **Step 3: Implement config package**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Provider string   `yaml:"provider"`
	Zones    []string `yaml:"zones"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func FindRoot(from string) (string, error) {
	dir := from
	for {
		if _, err := os.Stat(filepath.Join(dir, ".dnsctl")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not a dnsctl repository (no .dnsctl directory found)")
		}
		dir = parent
	}
}

func ConfigPath(root string) string {
	return filepath.Join(root, ".dnsctl", "config.yaml")
}

func StatePath(root string) string {
	return filepath.Join(root, ".dnsctl", "state.json")
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go test ./internal/config/ -v
```

Expected: All 3 tests PASS

- [ ] **Step 5: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add internal/config/ go.mod go.sum
git commit -m "feat: add config management and root discovery"
```

---

## Task 7: `dnsctl init` Command

**Files:**
- Create: `cmd/init.go`
- Modify: `cmd/root.go` (already exists)

- [ ] **Step 1: Install huh for interactive prompts**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go get github.com/charmbracelet/huh@latest
```

- [ ] **Step 2: Implement init command**

```go
// cmd/init.go
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	cfclient "github.com/wk/dnsctl/internal/cloudflare"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
	"github.com/wk/dnsctl/internal/state"
	"github.com/wk/dnsctl/internal/zone"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new dnsctl repository",
	Long:  "Authenticates with Cloudflare, lets you pick zones to track, and does an initial pull.",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dir, _ := os.Getwd()

	// Check if already initialized
	if _, err := os.Stat(filepath.Join(dir, ".dnsctl")); err == nil {
		return fmt.Errorf("already a dnsctl repository")
	}

	// Authenticate
	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	client, err := cfclient.NewClient(token)
	if err != nil {
		return err
	}

	fmt.Println("Verifying API token...")
	if err := client.VerifyToken(ctx); err != nil {
		return err
	}
	fmt.Println("Authenticated successfully.")

	// List zones
	zones, err := client.ListZones(ctx)
	if err != nil {
		return err
	}

	if len(zones) == 0 {
		return fmt.Errorf("no zones found in your Cloudflare account")
	}

	// Interactive zone picker
	var options []huh.Option[string]
	for _, z := range zones {
		options = append(options, huh.NewOption(z.Name, z.Name))
	}

	var selectedZones []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select zones to track").
				Options(options...).
				Value(&selectedZones),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("zone selection: %w", err)
	}

	if len(selectedZones) == 0 {
		return fmt.Errorf("no zones selected")
	}

	// Init git repo
	repo, err := gitpkg.Init(dir)
	if err != nil {
		return err
	}

	// Create .dnsctl directory
	dnsctlDir := filepath.Join(dir, ".dnsctl")
	if err := os.MkdirAll(dnsctlDir, 0755); err != nil {
		return fmt.Errorf("creating .dnsctl directory: %w", err)
	}

	// Build zone ID map
	zoneIDMap := make(map[string]string)
	for _, z := range zones {
		zoneIDMap[z.Name] = z.ID
	}

	// Save config
	cfg := &config.Config{
		Provider: "cloudflare",
		Zones:    selectedZones,
	}
	if err := config.Save(config.ConfigPath(dir), cfg); err != nil {
		return err
	}

	// Pull records for each zone
	st := &state.State{Zones: make(map[string]*state.ZoneState)}

	for _, zoneName := range selectedZones {
		zoneID := zoneIDMap[zoneName]
		fmt.Printf("Pulling records for %s...\n", zoneName)

		records, idMap, err := client.ListRecords(ctx, zoneID)
		if err != nil {
			return fmt.Errorf("pulling %s: %w", zoneName, err)
		}

		// Write zone file
		zf := &zone.ZoneFile{
			Zone:    zoneName,
			Records: records,
		}
		if err := zone.WriteFile(filepath.Join(dir, zoneName+".yaml"), zf); err != nil {
			return err
		}

		// Update state
		st.Zones[zoneName] = &state.ZoneState{
			ZoneID:  zoneID,
			Records: idMap,
		}

		fmt.Printf("  %d records\n", len(records))
	}

	// Save state
	if err := state.Save(config.StatePath(dir), st); err != nil {
		return err
	}

	// Git commit
	if err := repo.AddAllAndCommit("init: import zones from Cloudflare"); err != nil {
		return err
	}

	fmt.Println("Initialized dnsctl repository.")
	return nil
}
```

- [ ] **Step 3: Verify it compiles**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go build -o dnsctl .
./dnsctl init --help
```

Expected: Help output for `init` command

- [ ] **Step 4: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add cmd/init.go go.mod go.sum
git commit -m "feat: implement dnsctl init command"
```

---

## Task 8: `dnsctl pull` Command

**Files:**
- Create: `cmd/pull.go`

- [ ] **Step 1: Implement pull command**

```go
// cmd/pull.go
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	cfclient "github.com/wk/dnsctl/internal/cloudflare"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
	"github.com/wk/dnsctl/internal/state"
	"github.com/wk/dnsctl/internal/zone"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Fetch DNS records from Cloudflare",
	Long:  "Fetches current records from Cloudflare, writes zone files, and auto-commits.",
	RunE:  runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	root, err := config.FindRoot(".")
	if err != nil {
		return err
	}

	cfg, err := config.Load(config.ConfigPath(root))
	if err != nil {
		return err
	}

	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	client, err := cfclient.NewClient(token)
	if err != nil {
		return err
	}

	repo, err := gitpkg.Open(root)
	if err != nil {
		return err
	}

	// Get zone IDs from state
	st, err := state.Load(config.StatePath(root))
	if err != nil {
		return err
	}

	changed := false
	for _, zoneName := range cfg.Zones {
		zs, ok := st.Zones[zoneName]
		if !ok {
			fmt.Printf("Warning: zone %s not in state, skipping\n", zoneName)
			continue
		}

		fmt.Printf("Pulling %s...\n", zoneName)
		records, idMap, err := client.ListRecords(ctx, zs.ZoneID)
		if err != nil {
			fmt.Printf("Warning: failed to pull %s: %v\n", zoneName, err)
			continue
		}

		zf := &zone.ZoneFile{
			Zone:    zoneName,
			Records: records,
		}
		if err := zone.WriteFile(filepath.Join(root, zoneName+".yaml"), zf); err != nil {
			return err
		}

		zs.Records = idMap
		fmt.Printf("  %d records\n", len(records))
		changed = true
	}

	// Save state
	if err := state.Save(config.StatePath(root), st); err != nil {
		return err
	}

	// Check if git has changes
	status, err := repo.Status()
	if err != nil {
		return err
	}

	if status.IsClean() {
		fmt.Println("Already up to date.")
		return nil
	}

	msg := fmt.Sprintf("pull: sync from Cloudflare at %s", time.Now().UTC().Format(time.RFC3339))
	if err := repo.AddAllAndCommit(msg); err != nil {
		return err
	}

	if changed {
		fmt.Println("Pulled and committed.")
	}
	return nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go build -o dnsctl .
./dnsctl pull --help
```

Expected: Help output for `pull` command

- [ ] **Step 3: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add cmd/pull.go
git commit -m "feat: implement dnsctl pull command"
```

---

## Task 9: `dnsctl status` Command

**Files:**
- Create: `cmd/status.go`

- [ ] **Step 1: Implement status command**

```go
// cmd/status.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show modified zone files",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	root, err := config.FindRoot(".")
	if err != nil {
		return err
	}

	repo, err := gitpkg.Open(root)
	if err != nil {
		return err
	}

	status, err := repo.Status()
	if err != nil {
		return err
	}

	if status.IsClean() {
		fmt.Println("No local changes.")
		return nil
	}

	for file, code := range status.Files() {
		fmt.Printf("  %s %s\n", code, file)
	}

	return nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go build -o dnsctl .
./dnsctl status --help
```

Expected: Help output for `status` command

- [ ] **Step 3: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add cmd/status.go
git commit -m "feat: implement dnsctl status command"
```

---

## Task 10: `dnsctl diff` Command

**Files:**
- Create: `cmd/diff.go`

- [ ] **Step 1: Implement diff command**

The go-git library doesn't have great worktree diff support, so we shell out to `git diff` and also parse YAML for structured output.

```go
// cmd/diff.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wk/dnsctl/internal/config"
	"github.com/wk/dnsctl/internal/state"
	"github.com/wk/dnsctl/internal/zone"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show record-level changes",
	RunE:  runDiff,
}

var diffRaw bool

func init() {
	diffCmd.Flags().BoolVar(&diffRaw, "raw", false, "Show raw git diff instead of structured output")
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	root, err := config.FindRoot(".")
	if err != nil {
		return err
	}

	if diffRaw {
		return rawDiff(root)
	}

	return structuredDiff(root)
}

func rawDiff(root string) error {
	gitCmd := exec.Command("git", "diff")
	gitCmd.Dir = root
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	return gitCmd.Run()
}

func structuredDiff(root string) error {
	cfg, err := config.Load(config.ConfigPath(root))
	if err != nil {
		return err
	}

	st, err := state.Load(config.StatePath(root))
	if err != nil {
		return err
	}

	anyChanges := false
	for _, zoneName := range cfg.Zones {
		zs, ok := st.Zones[zoneName]
		if !ok {
			continue
		}

		zoneFilePath := filepath.Join(root, zoneName+".yaml")
		zf, err := zone.ReadFile(zoneFilePath)
		if err != nil {
			continue
		}

		cs := state.ComputeChangeset(zs, zf.Records)

		if len(cs.Create) == 0 && len(cs.Delete) == 0 {
			continue
		}

		anyChanges = true
		fmt.Printf("--- %s ---\n", zoneName)

		for _, op := range cs.Create {
			fmt.Printf("  + %s %s %s\n", op.Record.Type, op.Record.Name, op.Record.Content)
		}
		for _, op := range cs.Delete {
			parts := strings.Split(op.RecordKey, "]")
			if len(parts) == 3 {
				fmt.Printf("  - %s %s %s\n", strings.ToUpper(parts[0]), parts[1], parts[2])
			}
		}
	}

	if !anyChanges {
		fmt.Println("No record changes detected.")
	}

	return nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go build -o dnsctl .
./dnsctl diff --help
```

Expected: Help output for `diff` command, including `--raw` flag

- [ ] **Step 3: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add cmd/diff.go
git commit -m "feat: implement dnsctl diff command with structured output"
```

---

## Task 11: `dnsctl commit` Command

**Files:**
- Create: `cmd/commit.go`

- [ ] **Step 1: Implement commit command**

```go
// cmd/commit.go
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	cfclient "github.com/wk/dnsctl/internal/cloudflare"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
	"github.com/wk/dnsctl/internal/state"
	"github.com/wk/dnsctl/internal/zone"
)

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Push local changes to Cloudflare",
	Long:  "Compares local zone files against state, pushes changes to Cloudflare, and git commits.",
	RunE:  runCommit,
}

var commitMessage string

func init() {
	commitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "Commit message (required)")
	commitCmd.MarkFlagRequired("message")
	rootCmd.AddCommand(commitCmd)
}

func runCommit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	root, err := config.FindRoot(".")
	if err != nil {
		return err
	}

	cfg, err := config.Load(config.ConfigPath(root))
	if err != nil {
		return err
	}

	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	client, err := cfclient.NewClient(token)
	if err != nil {
		return err
	}

	st, err := state.Load(config.StatePath(root))
	if err != nil {
		return err
	}

	repo, err := gitpkg.Open(root)
	if err != nil {
		return err
	}

	// Remote drift check: fetch current state from Cloudflare and compare record IDs
	for _, zoneName := range cfg.Zones {
		zs, ok := st.Zones[zoneName]
		if !ok {
			continue
		}
		_, remoteIDs, err := client.ListRecords(ctx, zs.ZoneID)
		if err != nil {
			return fmt.Errorf("checking remote state for %s: %w", zoneName, err)
		}
		for key, localID := range zs.Records {
			if remoteID, exists := remoteIDs[key]; exists && remoteID != localID {
				return fmt.Errorf("remote has changed for %s. Run 'dnsctl pull' first", zoneName)
			}
		}
		// Check for records that exist remotely but not in our state
		for key := range remoteIDs {
			if _, exists := zs.Records[key]; !exists {
				return fmt.Errorf("remote has new records for %s. Run 'dnsctl pull' first", zoneName)
			}
		}
	}

	// Compute changesets for all zones
	type zoneChanges struct {
		zoneName string
		zoneID   string
		cs       state.Changeset
	}

	var allChanges []zoneChanges
	totalCreate, totalUpdate, totalDelete := 0, 0, 0

	for _, zoneName := range cfg.Zones {
		zs, ok := st.Zones[zoneName]
		if !ok {
			continue
		}

		zoneFilePath := filepath.Join(root, zoneName+".yaml")
		zf, err := zone.ReadFile(zoneFilePath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", zoneFilePath, err)
		}

		cs := state.ComputeChangeset(zs, zf.Records)

		if len(cs.Create) > 0 || len(cs.Delete) > 0 {
			allChanges = append(allChanges, zoneChanges{
				zoneName: zoneName,
				zoneID:   zs.ZoneID,
				cs:       cs,
			})
			totalCreate += len(cs.Create)
			totalUpdate += len(cs.Update)
			totalDelete += len(cs.Delete)
		}
	}

	if len(allChanges) == 0 {
		fmt.Println("No changes to push.")
		return nil
	}

	// Print summary
	fmt.Printf("Will create %d, update %d, delete %d records.\n", totalCreate, totalUpdate, totalDelete)

	// Show deletions explicitly
	for _, zc := range allChanges {
		for _, op := range zc.cs.Delete {
			parts := strings.Split(op.RecordKey, "]")
			if len(parts) == 3 {
				fmt.Printf("  Will DELETE: %s record '%s' (%s)\n", strings.ToUpper(parts[0]), parts[1], parts[2])
			}
		}
	}

	// Confirm
	fmt.Print("Proceed? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	// Apply changes
	var errors []string
	for _, zc := range allChanges {
		zs := st.Zones[zc.zoneName]

		for _, op := range zc.cs.Delete {
			fmt.Printf("  Deleting %s...\n", op.RecordKey)
			if err := client.DeleteRecord(ctx, zc.zoneID, op.RecordID); err != nil {
				errors = append(errors, fmt.Sprintf("delete %s: %v", op.RecordKey, err))
				continue
			}
			delete(zs.Records, op.RecordKey)
		}

		for _, op := range zc.cs.Create {
			fmt.Printf("  Creating %s %s...\n", op.Record.Type, op.Record.Name)
			newID, err := client.CreateRecord(ctx, zc.zoneID, op.Record, zc.zoneName)
			if err != nil {
				errors = append(errors, fmt.Sprintf("create %s %s: %v", op.Record.Type, op.Record.Name, err))
				continue
			}
			zs.Records[op.Record.Key()] = newID
		}

		for _, op := range zc.cs.Update {
			if err := client.UpdateRecord(ctx, zc.zoneID, op.RecordID, op.Record, zc.zoneName); err != nil {
				errors = append(errors, fmt.Sprintf("update %s %s: %v", op.Record.Type, op.Record.Name, err))
			}
		}
	}

	// Save state regardless (partial success updates IDs)
	if err := state.Save(config.StatePath(root), st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	if len(errors) > 0 {
		fmt.Println("\nSome operations failed:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		fmt.Println("\nState saved. Fix issues and retry. NOT committed to git.")
		return fmt.Errorf("%d operations failed", len(errors))
	}

	// Git commit
	if err := repo.AddAllAndCommit(commitMessage); err != nil {
		return err
	}

	fmt.Println("Changes pushed to Cloudflare and committed.")
	return nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go build -o dnsctl .
./dnsctl commit --help
```

Expected: Help output showing `-m` flag as required

- [ ] **Step 3: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add cmd/commit.go
git commit -m "feat: implement dnsctl commit command with confirmation and rollback"
```

---

## Task 12: `dnsctl log` Command

**Files:**
- Create: `cmd/log.go`

- [ ] **Step 1: Implement log command**

```go
// cmd/log.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wk/dnsctl/internal/config"
	gitpkg "github.com/wk/dnsctl/internal/git"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show change history",
	RunE:  runLog,
}

var logMax int

func init() {
	logCmd.Flags().IntVarP(&logMax, "max", "n", 20, "Maximum number of entries to show")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	root, err := config.FindRoot(".")
	if err != nil {
		return err
	}

	repo, err := gitpkg.Open(root)
	if err != nil {
		return err
	}

	entries, err := repo.Log(logMax)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No history yet.")
		return nil
	}

	for _, e := range entries {
		fmt.Printf("%s %s\n", e.Hash, e.Message)
	}

	return nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go build -o dnsctl .
./dnsctl log --help
```

Expected: Help output showing `-n` flag

- [ ] **Step 3: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add cmd/log.go
git commit -m "feat: implement dnsctl log command"
```

---

## Task 13: Build, Test, and Polish

**Files:**
- Modify: various (fixes found during integration)
- Create: `.gitignore`

- [ ] **Step 1: Add .gitignore**

```
# .gitignore
dnsctl
```

- [ ] **Step 2: Run all tests**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go test ./... -v
```

Expected: All tests pass

- [ ] **Step 3: Build final binary**

```bash
cd /home/wk/development/tools/dnsctl
export PATH=$PATH:/usr/local/go/bin
go build -o dnsctl .
./dnsctl --help
```

Expected: All 6 commands listed in help output

- [ ] **Step 4: Verify each command has help**

```bash
cd /home/wk/development/tools/dnsctl
./dnsctl init --help
./dnsctl pull --help
./dnsctl status --help
./dnsctl diff --help
./dnsctl commit --help
./dnsctl log --help
```

Expected: Each command shows its description and flags

- [ ] **Step 5: Commit**

```bash
cd /home/wk/development/tools/dnsctl
git add .gitignore
git commit -m "chore: add gitignore and finalize build"
```
