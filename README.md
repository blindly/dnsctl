# dnsctl

Git-like CLI for DNS management via Cloudflare. Pull your DNS records as YAML files, edit them locally, view diffs, and push changes back — all with full version history powered by git.

## Install

Download the latest binary from [Releases](https://github.com/blindly/dnsctl/releases), or build from source:

```bash
go install github.com/blindly/dnsctl@latest
```

## Quick Start

```bash
# Set your Cloudflare API token (or put it in a .env file)
export CLOUDFLARE_API_TOKEN=your-token-here

# Initialize — picks zones, pulls records, creates git repo
dnsctl init

# Edit a zone file
vim example.com.yaml

# See what changed
dnsctl status
dnsctl diff

# Push changes to Cloudflare
dnsctl commit -m "add blog CNAME"

# View history
dnsctl log
```

## Commands

| Command | Description |
|---------|-------------|
| `dnsctl init` | Authenticate, select zones, pull records, create git repo |
| `dnsctl pull` | Fetch current records from Cloudflare and auto-commit |
| `dnsctl status` | Show modified zone files |
| `dnsctl diff` | Show record-level changes (`--raw` for git diff) |
| `dnsctl commit -m "msg"` | Push local changes to Cloudflare |
| `dnsctl log` | Show change history (`-n` to limit entries) |

## Zone File Format

Each zone is a YAML file named `<zone>.yaml`:

```yaml
zone: example.com
records:
  - type: A
    name: "@"
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
```

Records are sorted deterministically by type, then name, so diffs are always clean.

## Authentication

dnsctl reads `CLOUDFLARE_API_TOKEN` from the environment. It also auto-loads a `.env` file from the current directory if one exists.

Create a token at [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens) with **Zone:Read** and **DNS:Edit** permissions.

## How It Works

Under the hood, dnsctl manages a real git repo. Every `pull` and `commit` creates a git commit, so you get full history, diffs, and the ability to push your DNS config to a remote like any other repo.

```
my-dns/
├── .git/                    # real git repo
├── .dnsctl/
│   ├── config.yaml          # tracked zones
│   └── state.json           # Cloudflare record ID mappings
├── example.com.yaml
└── another.dev.yaml
```

## Safety

- `dnsctl commit` checks for remote drift before applying changes — if someone changed DNS outside of dnsctl, you'll be asked to `pull` first
- Deletions are shown explicitly in the confirmation prompt
- Partial failures don't corrupt state — only successful changes are recorded

## License

MIT
