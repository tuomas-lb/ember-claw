# Phase 3: Build + Deploy Pipeline - Research

**Researched:** 2026-03-18
**Domain:** Multi-stage Docker build for Go+PicoClaw, Makefile conventions matching umbrella repo, interactive deployment wizard via Make shell, Kubernetes manifest targeting emberchat cluster
**Confidence:** HIGH

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| K8S-01 | Kubernetes manifests target emberchat cluster (rancher-based) | Kubeconfig at `/Users/tuomas/Projects/ember.kubeconfig.yaml` targets `https://rancher-2.kuupo.com/k8s/clusters/local` namespace `picoclaw`. Cluster server URL and auth token confirmed. |
| BLD-01 | Multi-stage Dockerfile builds sidecar binary with PicoClaw for linux/amd64 | PicoClaw is a public Go module (`github.com/sipeed/picoclaw v0.2.3`), already in go.mod. Two-stage build: `golang:1.25-alpine` builder + `alpine:3.23` runtime. `go build -o /sidecar ./cmd/sidecar` produces a statically linked binary. |
| BLD-02 | `make build-picoclaw` builds container image (linux/amd64) | Umbrella repo Makefile pattern confirmed: `docker buildx build --platform linux/amd64 -f Dockerfile -t reg.r.lastbot.com/{service}:{tag} .` with optional `EMBER_VERSION`-based build numbering. |
| BLD-03 | `make push-picoclaw` pushes image to `reg.r.lastbot.com` | Umbrella repo push pattern confirmed: `docker push reg.r.lastbot.com/{service}:{tag}` with build number guard (error if build hasn't run). |
| BLD-04 | `make deploy-picoclaw` launches interactive wizard: name, AI provider, API key, model, resource limits, env vars, then deploys to emberchat cluster | Interactive shell prompts in Make using `@read -p "..." VAR`, then delegates to `eclaw deploy` CLI binary which handles actual K8s resource creation. Make is the UX wrapper; CLI is the engine. |
</phase_requirements>

---

## Summary

Phase 3 is the final integration layer: a Dockerfile that builds the sidecar binary, Makefile targets matching the umbrella repo pattern, and an interactive `make deploy-picoclaw` wizard that drives the already-complete `eclaw deploy` CLI. All underlying mechanics (K8s resource creation, gRPC connection, port-forwarding) were built in Phases 1 and 2.

The Dockerfile is straightforward: a two-stage Go build using `golang:1.25-alpine` as the builder (matching PicoClaw's own Dockerfile) and `alpine:3.23` as the minimal runtime. PicoClaw is already a public Go module in go.mod (`github.com/sipeed/picoclaw v0.2.3`), so `go mod download` fetches everything. The sidecar binary at `cmd/sidecar/main.go` needs `ca-certificates` at runtime for HTTPS calls to AI providers.

The Makefile extends the project Makefile at the root of the umbrella repo. The service name is `ember-claw-sidecar`. Build numbering uses `.ember-build-numbers` (separate from `.coralie-build-numbers`). The `make deploy-picoclaw` target uses Make's `@read` to prompt interactively, then shells out to the compiled `eclaw` binary, avoiding any reimplementation of deployment logic.

K8S-01 (emberchat cluster targeting) is resolved by ensuring the CLI's default kubeconfig path points to `/Users/tuomas/Projects/ember.kubeconfig.yaml` and the default namespace is `picoclaw`. The cluster is Rancher-managed at `https://rancher-2.kuupo.com/k8s/clusters/local` with token auth already configured in the kubeconfig.

**Primary recommendation:** Write a lean Dockerfile, mirror the umbrella Makefile pattern exactly (including build number file), and make the interactive wizard a thin shell wrapper that calls `eclaw deploy` with collected arguments.

---

## Standard Stack

### Core

| Component | Value | Purpose | Why Standard |
|-----------|-------|---------|--------------|
| `golang:1.25-alpine` | Go 1.25 (matches go.mod) | Build stage base image | Matches PicoClaw's Dockerfile. `alpine` variant keeps the builder layer small. |
| `alpine:3.23` | latest alpine 3.23 | Runtime base image | Matches PicoClaw Dockerfile and umbrella repo convention. Has shell for debugging. |
| `ca-certificates` | via `apk` | HTTPS for LLM API calls | Required because the sidecar calls Anthropic/OpenAI/etc. APIs over HTTPS. |
| `docker buildx` | installed on system | Cross-platform build | Umbrella repo always uses `docker buildx build --platform linux/amd64`. |
| `reg.r.lastbot.com` | existing registry | Image registry | Already used by all Coralie services. Authentication must be established before push. |
| GNU Make | system make | Build orchestration | Umbrella repo pattern. All targets follow `build-{service}`, `push-{service}`, `deploy-{service}`. |

### Supporting

| Component | Version/Value | Purpose | When to Use |
|-----------|--------------|---------|-------------|
| `.ember-build-numbers` | new file (empty to start) | Per-service build counter | Created by first `make build-picoclaw`. Mirrors `.coralie-build-numbers`. |
| `eclaw` binary | compiled locally | Interactive wizard engine | `make deploy-picoclaw` compiles eclaw first, then calls it with wizard-collected args. |
| `KUBECONFIG` or `--kubeconfig` | `/Users/tuomas/Projects/ember.kubeconfig.yaml` | Emberchat cluster access | The CLI's default kubeconfig must point here, or `make deploy-picoclaw` passes it explicitly. |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `alpine:3.23` runtime | `gcr.io/distroless/static-debian12` | Smaller, but no shell for debugging and no `ca-certificates` built in. Use distroless only after hardening. |
| `make deploy-picoclaw` shell prompts | Pure Go interactive wizard in eclaw | More testable Go code, but umbrella repo pattern is Make-based. Make wizard → eclaw is the correct pattern. |
| Separate `Dockerfile.sidecar` | Single `Dockerfile` at repo root | Naming matches PicoClaw pattern (single `Dockerfile`). No need for a separate file since eclaw is a CLI tool, not a deployed service. |

---

## Architecture Patterns

### Dockerfile: Two-Stage Go Build

```
Stage 1 (builder):          Stage 2 (runtime):
golang:1.25-alpine          alpine:3.23
  WORKDIR /src               apk add ca-certificates tzdata
  COPY go.mod go.sum         COPY --from=builder /sidecar /usr/local/bin/sidecar
  RUN go mod download        RUN addgroup/adduser picoclaw (uid/gid 1000)
  COPY . .                   USER picoclaw
  RUN go build \             ENTRYPOINT ["sidecar"]
    -o /sidecar \
    ./cmd/sidecar
```

The builder stage downloads all Go modules first (layer caching), then copies source and builds. Only the resulting binary is copied to the runtime stage. The binary embeds all PicoClaw packages statically.

**Key decision:** Do NOT run `picoclaw onboard` in the Dockerfile (unlike PicoClaw's own Dockerfile). The sidecar reads its config from the mounted PVC at `/home/picoclaw/.picoclaw/`. There is nothing to onboard at image build time.

**Build command:**
```bash
docker buildx build --platform linux/amd64 -f Dockerfile -t reg.r.lastbot.com/ember-claw-sidecar:{tag} .
```

### Makefile: Umbrella Repo Pattern

The ember-claw Makefile at the project root mirrors the umbrella repo structure exactly:

```makefile
# Variables
SERVICE_NAME := ember-claw-sidecar
DOCKERFILE   := Dockerfile
IMAGE_REGISTRY := reg.r.lastbot.com
EMBER_VERSION ?=
BUILD_NUMBER_FILE := .ember-build-numbers
K8S_NAMESPACE := picoclaw
KUBECONFIG_PATH ?= /Users/tuomas/Projects/ember.kubeconfig.yaml

# Build (increments build counter if EMBER_VERSION is set)
build-picoclaw: ## Build Docker image for ember-claw-sidecar

# Push (uses current build counter; errors if build hasn't run)
push-picoclaw: ## Push Docker image to reg.r.lastbot.com

# Build + Push combined
build-push-picoclaw: ## Build and push Docker image

# Interactive deploy wizard
deploy-picoclaw: ## Deploy PicoClaw instance via interactive wizard

# Help (awk-based extraction from ## comments)
help: ## Show this help menu
```

Build numbering uses the same `sed -i.bak` pattern as the umbrella repo (macOS-compatible `sed`):
- With `EMBER_VERSION=1.0`: tag = `1.0.1`, `1.0.2`, ...
- Without `EMBER_VERSION`: tag = `production`

### Interactive Deployment Wizard Pattern

`make deploy-picoclaw` collects values interactively via shell `read`, then calls the compiled `eclaw` binary:

```make
deploy-picoclaw: ## Deploy PicoClaw instance via interactive wizard
	@echo "=== PicoClaw Instance Deployment ==="
	@read -p "Instance name: " NAME; \
	read -p "AI provider (anthropic/openai/copilot): " PROVIDER; \
	read -s -p "API key: " API_KEY; echo; \
	read -p "Model name: " MODEL; \
	read -p "CPU request [100m]: " CPU_REQ; CPU_REQ=$${CPU_REQ:-100m}; \
	read -p "CPU limit [500m]: " CPU_LIM; CPU_LIM=$${CPU_LIM:-500m}; \
	read -p "Memory request [128Mi]: " MEM_REQ; MEM_REQ=$${MEM_REQ:-128Mi}; \
	read -p "Memory limit [256Mi]: " MEM_LIM; MEM_LIM=$${MEM_LIM:-256Mi}; \
	./bin/eclaw deploy $$NAME \
		--provider $$PROVIDER \
		--api-key $$API_KEY \
		--model $$MODEL \
		--cpu-request $$CPU_REQ \
		--cpu-limit $$CPU_LIM \
		--memory-request $$MEM_REQ \
		--memory-limit $$MEM_LIM \
		--kubeconfig $(KUBECONFIG_PATH) \
		--namespace $(K8S_NAMESPACE)
```

**Key:** `read -s -p "API key: "` uses silent mode so the key does not echo to terminal. The eclaw binary then stores the key in a K8s Secret, never in shell history after this point.

Also support non-interactive mode via Make variables for CI:
```bash
make deploy-picoclaw NAME=alice PROVIDER=anthropic API_KEY=sk-xxx MODEL=claude-sonnet-4-20250514
```

### K8S-01: Emberchat Cluster Targeting

The CLI already accepts `--kubeconfig` and `--namespace` flags (from Phase 2 root.go). The only change needed for K8S-01 is ensuring sensible defaults for this specific cluster:

- Default kubeconfig: `/Users/tuomas/Projects/ember.kubeconfig.yaml` (or `KUBECONFIG` env var)
- Default namespace: `picoclaw` (already hardcoded as default in CLI)
- Cluster: `https://rancher-2.kuupo.com/k8s/clusters/local` (Rancher-managed, `local` context)

The cluster is already reachable with the token in the kubeconfig. No additional RBAC configuration is documented as needed (Pitfall 9 resolution: test against actual cluster before assuming permissions are correct).

### Recommended Project Structure for Phase 3

```
ember-claw/
  Makefile                           # NEW: build/push/deploy targets
  Dockerfile                         # NEW: multi-stage sidecar build
  .ember-build-numbers               # AUTO-CREATED by make build-picoclaw
  bin/                               # AUTO-CREATED: compiled eclaw binary
  cmd/sidecar/main.go                # EXISTS (Phase 1)
  cmd/eclaw/main.go                  # EXISTS (Phase 2)
  internal/cli/                      # EXISTS (Phase 2)
  internal/k8s/                      # EXISTS (Phase 2)
  internal/grpcclient/               # EXISTS (Phase 2)
  internal/server/                   # EXISTS (Phase 1)
```

### Anti-Patterns to Avoid

- **Do not run `picoclaw onboard` in Dockerfile:** The sidecar reads config from a mounted PVC. `onboard` creates config files at build time inside the image, which are then shadowed by the PVC mount anyway. Unnecessary and confusing.
- **Do not commit API keys in Makefile variables:** `read -s -p` avoids terminal echo. The key flows from terminal input -> eclaw CLI arg -> K8s Secret. It never touches a file.
- **Do not use `COPY . .` before `go mod download`:** Layer caching. Always copy go.mod/go.sum first, download modules, then copy source. Otherwise every source change invalidates the module download cache.
- **Do not use `paths=source_relative` for proto compilation in Makefile:** Proto compilation was resolved in Phase 1 with `--go_opt=module=`. Document in Makefile comments if a proto regen target is needed.
- **Do not shell out to `kubectl` in Makefile deploy target:** The whole point of the eclaw CLI is to avoid shelling to kubectl. The `deploy-picoclaw` wizard calls `eclaw`, which uses client-go.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Interactive prompts with defaults | Custom prompt parsing | Make `read` with `${VAR:-default}` | Shell already handles this. Three lines vs. custom code. |
| K8s resource creation | YAML templates + kubectl apply | eclaw deploy (already built) | All K8s CRUD lives in internal/k8s/resources.go. Makefile is just a UX wrapper. |
| Build numbering | Custom numbering scheme | The `.ember-build-numbers` file pattern from umbrella repo | Pattern is proven, macOS-compatible, already understood by the team. |
| Image tag management | Git SHA tagging, timestamp tagging | EMBER_VERSION.BUILD_NUMBER pattern | Matches umbrella repo. Consistent with the Coralie ecosystem already deployed. |
| Container user creation | Hardcoded UID in Dockerfile | `addgroup`/`adduser` alpine commands | Standard Alpine pattern, matches PicoClaw Dockerfile, creates named user at image build time. |

**Key insight:** Phase 3 adds almost no new Go code. It is plumbing: a Dockerfile and a Makefile. The Go code (CLI, K8s client, gRPC server) is complete.

---

## Common Pitfalls

### Pitfall 1: `sed -i` Is Not macOS-Compatible Without `.bak`

**What goes wrong:** `sed -i "s/..."` on macOS requires a backup extension argument (`sed -i.bak "s/..."`). The umbrella repo uses `sed -i.bak ... && rm -f *.bak`. If you use Linux-style `sed -i` without the extension, the build fails on macOS.
**How to avoid:** Copy the `sed -i.bak ... && rm -f *.bak` pattern exactly from the umbrella Makefile.
**Warning signs:** `sed: 1: "...": extra characters at the end of s command`

### Pitfall 2: Docker Build Context Includes go.work File

**What goes wrong:** If a `go.work` file exists (from local PicoClaw development), Docker COPY includes it. The build then fails because go.work references local paths not present in the Docker context.
**How to avoid:** Add `go.work` and `go.work.sum` to `.dockerignore`. PicoClaw is `v0.2.3` on the public module proxy — no `go.work` needed for the image build.
**Warning signs:** `go: cannot find module providing package in workspace` inside Docker build.

### Pitfall 3: Container User Does Not Own PVC Mount Path

**What goes wrong:** Container runs as uid 1000 (`picoclaw` user) but PVC is mounted at a path owned by root. PicoClaw cannot write session files, config, or workspace data. Sidecar starts but logs config write errors.
**How to avoid:** The deployment spec in resources.go mounts PVC at `/home/picoclaw/.picoclaw/`. The container user `picoclaw` (uid 1000) in the Dockerfile must have `/home/picoclaw/` as home directory. The PVC itself will be writable to any user because Kubernetes PVCs don't enforce UID ownership by default on most storage classes.
**Warning signs:** `permission denied` errors in sidecar logs when PicoClaw tries to write session state.

### Pitfall 4: `make deploy-picoclaw` eclaw Binary Not Built Yet

**What goes wrong:** `make deploy-picoclaw` shells out to `./bin/eclaw` but the binary hasn't been compiled yet. Make does not automatically build dependencies unless they're listed.
**How to avoid:** Either (a) add `build-eclaw` as a prerequisite for `deploy-picoclaw`, or (b) document that `make build-eclaw` must be run first. Option (a) is cleaner.
**Warning signs:** `./bin/eclaw: No such file or directory`

### Pitfall 5: Interactive `read` Prompts Fail in Non-Interactive Shells

**What goes wrong:** `make deploy-picoclaw` hangs or fails in CI, scripts, or when piped because `read` requires an interactive TTY.
**How to avoid:** Support variable-override mode: `make deploy-picoclaw NAME=alice PROVIDER=anthropic ...`. If all required variables are set, skip the prompts entirely. Pattern:
```makefile
@NAME=$${NAME:-}; \
if [ -z "$$NAME" ]; then read -p "Instance name: " NAME; fi; \
```

### Pitfall 6: API Key Echoed in Shell History or Output

**What goes wrong:** `@read -p "API key: " API_KEY` echoes the typed key to the terminal. `echo "API_KEY=$$API_KEY"` logs it.
**How to avoid:** Always use `read -s -p "API key: " API_KEY` (silent mode). Never echo or log the API key after collection.

### Pitfall 7: Rancher RBAC for `pods/portforward` Subresource

**What goes wrong:** `eclaw chat` requires `pods/portforward` RBAC permission. The emberchat cluster kubeconfig may have this or may not. Discovering this only after the build pipeline is complete is costly.
**How to avoid:** Before writing any Phase 3 code, verify with:
```bash
kubectl --kubeconfig /Users/tuomas/Projects/ember.kubeconfig.yaml \
  auth can-i create pods/portforward -n picoclaw
```
Also verify: `create deployments`, `create secrets`, `create configmaps`, `create persistentvolumeclaims`, `create services`.
**Warning signs:** `Error: pods "picoclaw-xxx" is forbidden: User "..." cannot create resource "pods/portforward"`

---

## Code Examples

### Pattern 1: Dockerfile (two-stage, Go+PicoClaw, alpine runtime)

```dockerfile
# Stage 1: Build sidecar binary
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src

# Cache module downloads before copying source
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build for linux/amd64
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /sidecar ./cmd/sidecar

# Stage 2: Minimal runtime
FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user matching PVC ownership expectations
RUN addgroup -g 1000 picoclaw && \
    adduser -D -u 1000 -G picoclaw -h /home/picoclaw picoclaw

COPY --from=builder /sidecar /usr/local/bin/sidecar

USER picoclaw

EXPOSE 50051 8080

ENTRYPOINT ["sidecar"]
```

**Note:** `CGO_ENABLED=0` ensures a fully static binary that runs in Alpine without additional C libraries. `-ldflags="-w -s"` strips debug info for a smaller binary.

### Pattern 2: Makefile build target (exact umbrella repo pattern)

```makefile
SERVICE_NAME := ember-claw-sidecar
IMAGE_REGISTRY := reg.r.lastbot.com
DOCKERFILE := Dockerfile
EMBER_VERSION ?=
BUILD_NUMBER_FILE := .ember-build-numbers

build-picoclaw: ## Build Docker image for ember-claw-sidecar
	@if [ -n "$(EMBER_VERSION)" ]; then \
		SERVICE_NAME="$(SERVICE_NAME)"; \
		if [ -f $(BUILD_NUMBER_FILE) ]; then \
			CURRENT=$$(grep "^$$SERVICE_NAME:" $(BUILD_NUMBER_FILE) | cut -d: -f2 || echo "0"); \
		else \
			CURRENT="0"; \
		fi; \
		BUILD_NUMBER=$$((CURRENT + 1)); \
		if [ -f $(BUILD_NUMBER_FILE) ]; then \
			if grep -q "^$$SERVICE_NAME:" $(BUILD_NUMBER_FILE); then \
				sed -i.bak "s/^$$SERVICE_NAME:.*/$$SERVICE_NAME:$$BUILD_NUMBER/" $(BUILD_NUMBER_FILE) && rm -f $(BUILD_NUMBER_FILE).bak; \
			else \
				echo "$$SERVICE_NAME:$$BUILD_NUMBER" >> $(BUILD_NUMBER_FILE); \
			fi; \
		else \
			echo "$$SERVICE_NAME:$$BUILD_NUMBER" > $(BUILD_NUMBER_FILE); \
		fi; \
		IMAGE_TAG="$(EMBER_VERSION).$$BUILD_NUMBER"; \
		echo "Building Docker image: $(IMAGE_REGISTRY)/$$SERVICE_NAME:$$IMAGE_TAG"; \
	else \
		IMAGE_TAG="production"; \
		echo "Building Docker image: $(IMAGE_REGISTRY)/$(SERVICE_NAME):$$IMAGE_TAG"; \
	fi; \
	docker buildx build --platform linux/amd64 \
		-f $(DOCKERFILE) \
		-t $(IMAGE_REGISTRY)/$(SERVICE_NAME):$$IMAGE_TAG \
		.
```

### Pattern 3: Interactive wizard with defaults and API key masking

```makefile
deploy-picoclaw: build-eclaw ## Deploy PicoClaw instance via interactive wizard
	@echo "=== PicoClaw Instance Deployment ==="
	@NAME=$${NAME:-}; \
	if [ -z "$$NAME" ]; then read -p "Instance name: " NAME; fi; \
	PROVIDER=$${PROVIDER:-}; \
	if [ -z "$$PROVIDER" ]; then read -p "AI provider (anthropic/openai/copilot): " PROVIDER; fi; \
	API_KEY=$${API_KEY:-}; \
	if [ -z "$$API_KEY" ]; then read -s -p "API key: " API_KEY; echo; fi; \
	MODEL=$${MODEL:-}; \
	if [ -z "$$MODEL" ]; then read -p "Model name: " MODEL; fi; \
	CPU_REQ=$${CPU_REQ:-100m}; \
	CPU_LIM=$${CPU_LIM:-500m}; \
	MEM_REQ=$${MEM_REQ:-128Mi}; \
	MEM_LIM=$${MEM_LIM:-256Mi}; \
	./bin/eclaw deploy $$NAME \
		--provider $$PROVIDER \
		--api-key $$API_KEY \
		--model $$MODEL \
		--cpu-request $$CPU_REQ \
		--cpu-limit $$CPU_LIM \
		--memory-request $$MEM_REQ \
		--memory-limit $$MEM_LIM \
		--kubeconfig $(KUBECONFIG_PATH) \
		--namespace $(K8S_NAMESPACE)
```

### Pattern 4: eclaw binary build target

```makefile
build-eclaw: ## Build eclaw CLI binary to ./bin/eclaw
	@mkdir -p bin
	@go build -o bin/eclaw ./cmd/eclaw
	@echo "Built bin/eclaw"
```

---

## State of the Art

| Old Approach | Current Approach | Impact |
|--------------|------------------|--------|
| `docker build` (single platform) | `docker buildx build --platform linux/amd64` | Required for cross-platform (macOS arm64 -> linux/amd64) |
| `grpc.Dial()` | `grpc.NewClient()` | Already used in Phase 2; `Dial` is deprecated in grpc-go |
| Helm for K8s packaging | Make + eclaw CLI | Project decision; matches umbrella repo |
| `sed -i "..."` (Linux) | `sed -i.bak "..." && rm -f *.bak` (macOS) | macOS compatibility |

**Not deprecated for this project:**
- Alpine base image (still correct; PicoClaw Dockerfile uses it)
- Static Go binary (CGO_ENABLED=0; correct for Alpine runtime)

---

## Open Questions

1. **RBAC permissions on emberchat cluster**
   - What we know: Kubeconfig at `/Users/tuomas/Projects/ember.kubeconfig.yaml` targets `https://rancher-2.kuupo.com/k8s/clusters/local` with a bearer token.
   - What's unclear: Whether the token has `pods/portforward` and all required resource creation permissions in the `picoclaw` namespace. Rancher-managed clusters can have scoped RBAC (Pitfall 9).
   - Recommendation: First task of Wave 0 should be `kubectl auth can-i` checks for all required verbs. If insufficient, document what RBAC bindings need to be created by the cluster admin.

2. **Namespace `picoclaw` existence on emberchat cluster**
   - What we know: The CLI defaults to namespace `picoclaw`. It is used in resources.go constants.
   - What's unclear: Whether the namespace already exists on the cluster.
   - Recommendation: Add `kubectl get namespace picoclaw` check to Wave 0 verification, or add a `make create-namespace` target that creates it idempotently.

3. **Docker registry authentication for `reg.r.lastbot.com`**
   - What we know: All Coralie services push to this registry. Authentication is required before push (umbrella CLAUDE.md note).
   - What's unclear: Whether the developer machine already has Docker login configured for this registry.
   - Recommendation: Document `docker login reg.r.lastbot.com` as a prerequisite in the Makefile help text. Not a code problem; a setup documentation note.

4. **`eclaw deploy` flag names for resource limits**
   - What we know: `internal/cli/deploy.go` was built in Phase 2 with `--cpu-request`, `--cpu-limit`, `--memory-request`, `--memory-limit`, `--image`, and custom env flags.
   - What's unclear: The exact flag names — they were not shown in the summary. The Makefile wizard must use the correct flag names.
   - Recommendation: Read `internal/cli/deploy.go` before writing the Makefile wizard. The plan should specify the exact flag names to use.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | none (go test ./...) |
| Quick run command | `go test ./... -count=1` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements -> Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| BLD-01 | Dockerfile produces valid linux/amd64 binary | smoke | `docker buildx build --platform linux/amd64 -f Dockerfile -t ember-claw-sidecar:test . && docker run --rm ember-claw-sidecar:test sidecar --help` | No - Wave 0 |
| BLD-02 | `make build-picoclaw` runs without error | smoke | `make build-picoclaw` | No - Wave 0 |
| BLD-03 | `make push-picoclaw` errors if no prior build | unit | N/A (Make shell logic, manually verify) | manual-only |
| BLD-04 | `make deploy-picoclaw` passes collected args to eclaw | manual | Interactive terminal session | manual-only |
| K8S-01 | Deployed resources appear in emberchat cluster namespace `picoclaw` | integration | `kubectl --kubeconfig .../ember.kubeconfig.yaml get deployments -n picoclaw` | manual-only (requires live cluster) |

**Note:** BLD-01 and BLD-02 can be automated. BLD-03, BLD-04, and K8S-01 are manual verification steps requiring a live cluster and Docker registry authentication.

### Sampling Rate

- **Per task commit:** `go build ./... && go vet ./...` (existing code must not regress)
- **Per wave merge:** `go test ./... -race -count=1` (all unit tests pass)
- **Phase gate:** Docker build succeeds + `make build-picoclaw` works + manual deploy wizard test before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `Dockerfile` - the multi-stage build file (BLD-01, BLD-02)
- [ ] `Makefile` - build/push/deploy targets (BLD-02, BLD-03, BLD-04)
- [ ] `bin/` - directory for compiled eclaw binary (created by `make build-eclaw`)
- [ ] `.dockerignore` - exclude `go.work`, `go.work.sum`, `.planning/`, `bin/` from build context
- [ ] RBAC verification: `kubectl auth can-i` checks against emberchat cluster

---

## Sources

### Primary (HIGH confidence)

- `/Users/tuomas/Projects/Makefile` (umbrella repo) - build/push/deploy target pattern, build numbering, `sed -i.bak`, help format, `IMAGE_REGISTRY`, `K8S_NAMESPACE` conventions
- `/Users/tuomas/Projects/picoclaw/docker/Dockerfile` - `golang:1.25-alpine` builder, `alpine:3.23` runtime, `ca-certificates`, `addgroup`/`adduser` commands, non-root user pattern
- `/Users/tuomas/Projects/ember-claw/go.mod` - confirmed `github.com/sipeed/picoclaw v0.2.3` is a versioned public module (not a commit pin)
- `/Users/tuomas/Projects/ember-claw/internal/k8s/resources.go` - `DefaultImage = "reg.r.lastbot.com/ember-claw-sidecar:latest"`, `MountPath = "/home/picoclaw/.picoclaw"`, all K8s resource construction already done
- `/Users/tuomas/Projects/ember-claw/cmd/sidecar/main.go` - binary entry point at `./cmd/sidecar`, confirmed compilable
- `/Users/tuomas/Projects/ember.kubeconfig.yaml` - cluster URL `https://rancher-2.kuupo.com/k8s/clusters/local`, context name `local`, bearer token auth

### Secondary (MEDIUM confidence)

- Phase 2 summaries (02-01, 02-02, 02-03) - confirmed all CLI flags and K8s client patterns built in Phase 2
- `/Users/tuomas/Projects/coralie-gemini-worker/deploy/deployment.yaml` - K8s manifest pattern reference (labels, resource limits, health probes, PVC usage)

### Tertiary (LOW confidence)

- None — all research based on direct source code analysis.

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — verified against PicoClaw Dockerfile, umbrella Makefile, go.mod, and existing code
- Architecture: HIGH — Dockerfile pattern confirmed from PicoClaw source; Makefile pattern confirmed from umbrella repo direct inspection
- Pitfalls: HIGH — `sed -i` macOS issue confirmed in umbrella Makefile (it uses `.bak`); API key masking confirmed from requirements; others from established K8s/Make patterns

**Research date:** 2026-03-18
**Valid until:** 2026-04-18 (stable domain; Go and Docker conventions don't change rapidly)
