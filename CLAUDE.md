# CLAUDE.md — EmberClaw

## Project Overview

EmberClaw (`eclaw`) is a Go CLI tool for deploying and managing PicoClaw AI assistant instances on Kubernetes. Each instance runs as a single-container pod with a gRPC sidecar, persistent storage (PVC), and configurable MCP integrations.

## Stack

- **Language:** Go 1.25
- **CLI framework:** cobra + fatih/color
- **Kubernetes:** client-go (in-process, no kubectl dependency)
- **gRPC:** protobuf-generated client/server for chat
- **Container:** Debian bookworm-slim with Python, Node.js, Go, cloud CLIs
- **Build:** Makefile + Docker buildx (linux/amd64)

## Key Directories

| Path | Purpose |
|------|---------|
| `cmd/eclaw/` | CLI entrypoint |
| `cmd/sidecar/` | Container entrypoint (gRPC server + PicoClaw agent) |
| `internal/cli/` | Cobra command implementations |
| `internal/k8s/` | Kubernetes client, resource management, labels |
| `internal/providers/` | AI provider API key resolution + model listing |
| `internal/envfile/` | `.env` file parser |
| `internal/server/` | gRPC server implementation |
| `internal/grpcclient/` | gRPC client for chat |
| `internal/tools/` | Linear, Slack tool integrations |
| `docs/` | Deployment guide, architecture, tool development |
| `assets/brand/` | Logo and brand assets |

## Rules

### Documentation Rule (MANDATORY)

**Every feature, flag, config option, integration, or behavioral change MUST be documented before the commit is considered complete.** This includes:

1. **README.md** — user-facing feature docs, flag tables, examples
2. **docs/deployment-guide.md** — setup steps, `.env` variables, troubleshooting
3. **docs/architecture.md** — internal design, data flow, config structures
4. **CLAUDE.md** — project structure, conventions, rules

Never leave anything undocumented. If you add a flag, document it. If you add an env var, document it. If you change behavior, document it.

### Code Conventions

- All K8s resources use labels from `internal/k8s/labels.go` (managed-by, instance, name, component)
- Resource names follow pattern: `picoclaw-<instance-name>` (deployment, service, secret, configmap, PVC)
- Deploy uses upsert semantics (create-or-update) — never fails on "already exists"
- Namespace is auto-created if it doesn't exist
- Config changes trigger pod restart automatically
- `.env` is auto-loaded; existing env vars are NOT overridden

### Container Image

- Base: `debian:bookworm-slim` (glibc needed for bun/backlog.md)
- Pre-installed: curl, jq, git, gh, python3, nodejs, go, bun, gcloud, aws, az
- Pre-installed Python packages: requests, beautifulsoup4, pyyaml
- Pre-installed npm packages: backlog.md, caldav-mcp, @playwright/mcp (+ headless chromium at `/opt/playwright-browsers`)
- Bundled binaries: `sidecar` (entrypoint) and `eclaw` (fleet control, uses in-cluster ServiceAccount)
- System git credential helper authenticates `https://github.com` from `GITHUB_TOKEN` env
- `PIP_USER=1` + `PYTHONUSERBASE` on PVC for persistent pip packages
- `PIP_BREAK_SYSTEM_PACKAGES=1` for system-level pip access

### PicoClaw Config

Generated config.json sets container-optimized defaults:
- `restrict_to_workspace: false`
- `allow_read_outside_workspace: true`
- `max_tool_iterations: 50`
- `enable_deny_patterns: false` (safety guard off in container)
- `allow_remote: true`

### MCP Integrations (built into container)

| MCP Server | Package | Purpose | Config |
|------------|---------|---------|--------|
| `backlog` | backlog.md | Task management | Always enabled, uses workspace dir |
| `calendar-*` | caldav-mcp | CalDAV calendars | Via `--caldav` flag or `CALDAV_*` env vars |
| `gmail` | gmail-mcp (local) | IMAP email access | Via `eclaw set-gmail` command |
| `playwright` | @playwright/mcp | Headless browser automation | Via `--playwright` deploy flag |

### Web Control Interface

- Sidecar serves a control UI at `/` on port 8080 (status + chat) alongside `/health`/`/ready`
- `/api/status` + `/api/chat` require `Authorization: Bearer $CONTROL_TOKEN`; disabled (503) when the env var is unset
- Set the token with `eclaw set-secret <name> CONTROL_TOKEN <token>`; expose with `eclaw expose <name> --type ingress --host ... --tls`
- Channel-manager webhook server is not started (port 8080 collision; Telegram long-polls)

### Fleet & Storage Features

- `--fleet-admin` creates ServiceAccount/Role/RoleBinding `picoclaw-<name>-fleet` (namespace-scoped) and injects `ECLAW_NAMESPACE`/`ECLAW_IMAGE` so the in-container `eclaw` binary can manage sibling instances; cleaned up on delete
- `--shared-pvc <name>` mounts a fleet-shared PVC at `/home/picoclaw/shared` (`SHARED_DIR` env); created on demand, never deleted with an instance
- `--github-token` injects `GITHUB_TOKEN` + `GH_TOKEN` into the instance Secret

### Build & Deploy

```bash
make build-eclaw                              # Build CLI
make build-push-picoclaw EMBER_VERSION=0.1    # Build + push container image
eclaw deploy my-agent --provider gemini --model gemini-2.5-flash
```

Without `EMBER_VERSION`, image is tagged `:latest` only. With it, tagged as `<version>.<build>` AND `:latest`.

### Testing

```bash
go test ./...
```

Tests use fake K8s clientset — no real cluster needed.
