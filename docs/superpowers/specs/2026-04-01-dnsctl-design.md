# dnsctl - Git-like CLI for DNS Management

## Overview

A CLI tool that brings a Git-like workflow to DNS record management. Users can `pull` records from Cloudflare, edit them as YAML files, view `diff`s and `status`, and `commit` changes back. Built on real git for history, diffing, and audit trails.

**Target user:** Individual DevOps/infra engineers managing their own domains via Cloudflare.

**Language:** Go (single binary, cobra for CLI, go-git for git operations, cloudflare-go SDK).

**Auth:** `CLOUDFLARE_API_TOKEN` environment variable. No config file fallback.

## CLI Commands

| Command | Description |
|---------|-------------|
| `dnsctl init` | Init git repo, authenticate, select zones, initial pull |
| `dnsctl pull` | Fetch records from Cloudflare, write YAML files, auto-commit |
| `dnsctl status` | Show modified zone files (via git status) |
| `dnsctl diff` | Show record-level changes (via git diff) |
| `dnsctl commit -m "msg"` | Push local changes to Cloudflare, then git commit |
| `dnsctl log` | Show change history (via git log) |

## Zone File Format

One YAML file per zone, named `<zone>.yaml`. Records sorted deterministically by type then name.

```yaml
zone: example.com
records:
  - type: A
    name: "@"
    content: 192.0.2.1
    ttl: 300
    proxied: true

  - type: A
    name: www
    content: 192.0.2.1
    ttl: 300
    proxied: true

  - type: CNAME
    name: blog
    content: example.netlify.app
    ttl: 1
    proxied: false

  - type: MX
    name: "@"
    content: mail.example.com
    priority: 10
    ttl: 3600

  - type: TXT
    name: "@"
    content: "v=spf1 include:_spf.google.com ~all"
    ttl: 3600
```

Key conventions:
- `name: "@"` for zone apex
- `ttl: 1` means "automatic" in Cloudflare
- `proxied` field included for Cloudflare-specific behavior
- Cloudflare record IDs are NOT stored in zone files (kept in state file)

## Directory Structure (User's DNS Repo)

When a user runs `dnsctl init`:

```
my-dns/
├── .git/                     # real git repo
├── .dnsctl/
│   ├── config.yaml           # tracked zones and provider
│   └── state.json            # Cloudflare record ID mappings
├── example.com.yaml          # zone file
└── anotherdomain.dev.yaml    # zone file
```

### .dnsctl/config.yaml

```yaml
provider: cloudflare
zones:
  - example.com
  - anotherdomain.dev
```

### .dnsctl/state.json

Maps Cloudflare record IDs to local records. Used by `commit` to determine whether to create, update, or delete records via the API. Git-tracked so cloning preserves the mapping.

```json
{
  "example.com": {
    "records": {
      "a]@]192.0.2.1": "cf-record-id-abc123",
      "a]www]192.0.2.1": "cf-record-id-def456",
      "cname]blog]example.netlify.app": "cf-record-id-ghi789"
    },
    "zone_id": "cf-zone-id-xyz"
  }
}
```

Record keys are `type]name]content` (lowercased) to uniquely identify each record.

## Core Operations

### dnsctl init

1. Validate `CLOUDFLARE_API_TOKEN` with a test API call
2. `git init` in current directory
3. Fetch all zones from Cloudflare, present interactive multi-select picker
4. Write `.dnsctl/config.yaml` and `.dnsctl/state.json`
5. Pull all records for selected zones, write YAML files
6. `git add . && git commit -m "init: import zones from Cloudflare"`

### dnsctl pull

1. Read `.dnsctl/config.yaml` for tracked zones
2. Fetch all records for each zone from Cloudflare API
3. Overwrite YAML files with fresh data (deterministic sort)
4. Update `state.json` with current record IDs
5. If changes: `git add . && git commit -m "pull: sync from Cloudflare at <timestamp>"`
6. If no changes: print "Already up to date."

### dnsctl status

1. Run `git status --short` on zone files
2. Pretty-print modified files with record-level summary (e.g., "modified: example.com.yaml (2 records changed)")
3. If clean: "No local changes."

### dnsctl diff

1. Run `git diff` on zone files
2. Parse and present as structured record-level diff (e.g., `+ A blog 192.0.2.5` / `- CNAME blog example.netlify.app`)

### dnsctl commit -m "message"

1. Parse local YAML files, compare against `state.json`
2. Compute changeset: records to create, update, or delete
3. Print summary: "Will create 1, update 2, delete 0 records. Proceed? [y/N]"
4. On confirm: execute Cloudflare API calls
5. Update `state.json` with new record IDs
6. `git add . && git commit -m "<user message>"`
7. On API failure: do not git commit, update `state.json` only for succeeded records, report what failed

### dnsctl log

1. Run `git log --oneline`

## Error Handling

**Authentication:** Clear error message with link to Cloudflare token page if `CLOUDFLARE_API_TOKEN` is unset or invalid.

**Partial API failures during commit:** If some API calls succeed and others fail:
- Do NOT git commit (local state stays dirty for retry)
- Update `state.json` only for records that succeeded
- Print what succeeded and what failed

**Remote drift:** Before applying changes, `dnsctl commit` fetches current state from Cloudflare. If records changed remotely since last pull, abort with: "Remote has changed. Run 'dnsctl pull' first."

**Record deletion safety:** Deletions are highlighted explicitly in the confirmation prompt: "Will DELETE: A record 'staging' (192.0.2.99). Proceed? [y/N]"

**Invalid YAML:** Validate zone files before any API calls. Report line numbers for parse errors.

**Missing zones:** If a zone in `config.yaml` no longer exists in Cloudflare, warn during pull but continue with remaining zones.

## Project Structure (Go Source)

```
/home/wk/development/tools/dnsctl/
├── main.go
├── go.mod
├── go.sum
├── cmd/
│   ├── root.go          # cobra root command
│   ├── init.go          # dnsctl init
│   ├── pull.go          # dnsctl pull
│   ├── status.go        # dnsctl status
│   ├── diff.go          # dnsctl diff
│   ├── commit.go        # dnsctl commit
│   └── log.go           # dnsctl log
├── internal/
│   ├── cloudflare/      # Cloudflare API client wrapper
│   │   └── client.go
│   ├── git/             # git operations (via go-git)
│   │   └── repo.go
│   ├── zone/            # YAML zone file parsing/writing
│   │   └── zone.go
│   └── state/           # state.json management
│       └── state.go
└── docs/
    └── superpowers/
        └── specs/
            └── 2026-04-01-dnsctl-design.md
```

## Dependencies

- `github.com/spf13/cobra` — CLI framework
- `github.com/go-git/go-git/v5` — git operations
- `github.com/cloudflare/cloudflare-go` — Cloudflare API
- `gopkg.in/yaml.v3` — YAML parsing
- `github.com/charmbracelet/huh` — interactive terminal prompts (zone picker in init)

## What's Out of Scope (for now)

- Multi-provider support (Route53, etc.)
- Team collaboration / PR workflows
- CI/CD integration
- Remote git push (user can do this themselves)
- DNS record types beyond what Cloudflare supports
- Branching workflows for DNS changes
