---
phase: 02-cli-k8s-integration
plan: "01"
subsystem: infra
tags: [kubernetes, client-go, k8s, labels, fake-clientset, unit-testing]

# Dependency graph
requires:
  - phase: 01-proto-sidecar
    provides: "Go module structure, picoclaw dependency, grpc/proto generated code"
provides:
  - "internal/k8s/labels.go: label constants and selector helpers (InstanceLabels, InstanceSelector, ManagedSelector)"
  - "internal/k8s/client.go: K8s Client struct with NewClient and NewClientFromClientset constructors"
  - "internal/k8s/resources.go: full CRUD for PicoClaw instances (DeployInstance, ListInstances, DeleteInstance, DeletePVC, GetInstanceStatus, GetInstanceLogs, FindRunningPod)"
  - "Comprehensive unit tests using fake.NewSimpleClientset() proving all resource invariants"
affects: [02-02, 02-03, 02-04, 02-05, 02-06]

# Tech tracking
tech-stack:
  added:
    - "k8s.io/client-go v0.35.2 (Go MVS resolved from v0.33.0 request)"
    - "k8s.io/api v0.35.2"
    - "k8s.io/apimachinery v0.35.2"
    - "k8s.io/client-go/kubernetes/fake (test only)"
  patterns:
    - "Label-based K8s resource management: all eclaw resources carry app.kubernetes.io/managed-by=eclaw"
    - "API key in Secret StringData with envFrom.secretRef injection (never in Deployment env.value)"
    - "PVC preservation on delete: DeleteInstance removes compute resources, DeletePVC is explicit"
    - "NewClientFromClientset constructor for test isolation with fake clientset"
    - "TDD with RED commit before GREEN implementation"

key-files:
  created:
    - internal/k8s/client.go
    - internal/k8s/labels.go
    - internal/k8s/resources.go
    - internal/k8s/labels_test.go
    - internal/k8s/resources_test.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "client-go resolved to v0.35.2 by Go MVS despite requesting v0.33.0; no conflicts with picoclaw dep tree"
  - "kubernetes.Interface used in Client struct (not *kubernetes.Clientset) to allow fake clientset injection"
  - "DeleteInstance uses label-selector list-then-delete pattern (fake clientset does not support DeleteCollection in all versions)"
  - "ConfigMap created for custom env vars even when CustomEnv is empty (keeps Deployment spec consistent)"

patterns-established:
  - "Pattern: resourceName(name) = 'picoclaw-' + name -- all resources use this prefix"
  - "Pattern: Secret name = picoclaw-{name}-config, PVC = picoclaw-{name}-data, Deployment/Service = picoclaw-{name}"
  - "Pattern: PICOCLAW_PROVIDERS_{PROVIDER}_API_KEY format for provider API keys in Secrets"

requirements-completed: [CLI-01, CLI-02, CLI-03, CLI-04, CLI-05, K8S-02, K8S-03, CONF-01, CONF-02, CONF-03, CONF-04, CONF-05]

# Metrics
duration: 9min
completed: 2026-03-16
---

# Phase 2 Plan 01: K8s Client Abstraction Layer Summary

**K8s client-go CRUD layer for PicoClaw instances: deploy 5 resources (Secret/ConfigMap/PVC/Deployment/Service), list by managed-by label, delete compute while preserving PVC, and return deployment+pod status -- all verified by 14 fake-clientset unit tests**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-16T12:05:21Z
- **Completed:** 2026-03-16T12:14:42Z
- **Tasks:** 2 (each with TDD RED+GREEN)
- **Files modified:** 7

## Accomplishments

- client-go v0.35.2 added alongside picoclaw with zero dependency conflicts; `go build ./...` and `go vet ./...` both clean
- Full K8s resource CRUD: 7 exported methods on `*Client` covering deploy, list, delete, status, logs, and pod discovery
- 14 unit tests using `fake.NewSimpleClientset()` prove all resource invariants including API-key-in-Secret, PVC preservation, label filtering, and resource limits

## Task Commits

Each task was committed atomically (TDD: RED commit before GREEN):

1. **Task 1 RED: Failing label tests** - `2d8542e` (test)
2. **Task 1 GREEN: client-go deps, client.go, labels.go** - `a127ff0` (feat)
3. **Task 2 RED: Failing resource CRUD tests** - `f137603` (test)
4. **Task 2 GREEN: resources.go implementation** - `90279a8` (feat)

**Plan metadata:** (pending)

_Note: TDD tasks have separate test/feat commits per the TDD execution protocol_

## Files Created/Modified

- `internal/k8s/labels.go` - Label constants (LabelManagedBy/Instance/Name/Component, ManagedByValue/NameValue/ComponentValue) and helpers (InstanceLabels, InstanceSelector, ManagedSelector)
- `internal/k8s/client.go` - Client struct wrapping kubernetes.Interface + rest.Config + namespace; NewClient (kubeconfig path) and NewClientFromClientset (test injection)
- `internal/k8s/resources.go` - DeployOptions/InstanceSummary/InstanceStatus types; DeployInstance, ListInstances, DeleteInstance, DeletePVC, GetInstanceStatus, GetInstanceLogs, FindRunningPod
- `internal/k8s/labels_test.go` - 3 tests for label selector correctness
- `internal/k8s/resources_test.go` - 11 tests covering all CRUD invariants using fake clientset
- `go.mod` - Added k8s.io/client-go, k8s.io/api, k8s.io/apimachinery (resolved v0.35.2)
- `go.sum` - Updated checksums

## Decisions Made

- **client-go version resolved to v0.35.2:** Go MVS upgraded from requested v0.33.0 to v0.35.2 when resolving the full dependency graph. Build succeeded with no conflicts -- picoclaw and client-go coexist cleanly.
- **kubernetes.Interface instead of *kubernetes.Clientset:** Using the interface type in the Client struct allows `fake.NewSimpleClientset()` (which implements `kubernetes.Interface`) to be injected directly in tests without casts.
- **List-then-delete pattern in DeleteInstance:** The fake clientset's DeleteCollection support is unreliable across versions. Using List + individual Deletes is more portable and testable.
- **ConfigMap always created in DeployInstance:** Even when CustomEnv is empty, the ConfigMap is created to keep the Deployment spec stable (envFrom.configMapRef always present). This avoids conditional spec branches in future update operations.

## Deviations from Plan

None - plan executed exactly as written.

The only minor variance was client-go resolving to v0.35.2 instead of v0.33.0, which is expected Go MVS behavior (higher compatible version selected) and had no negative impact.

## Issues Encountered

None - no blocking issues. The `intstr.FromInt32` import for probe port specs was added during implementation (Rule 2: missing critical) but was a trivial single-import addition, not a significant deviation.

## User Setup Required

None - no external service configuration required. All tests run with fake clientset; no live cluster needed for this plan.

## Next Phase Readiness

- `internal/k8s/` package complete and tested -- ready for Plan 02-02 (Cobra CLI commands)
- CLI commands can import `k8s.Client` and call all 7 CRUD methods
- Port-forward (for `eclaw chat`) is deferred to the portforward.go file in a later plan
- All 12 requirements for this plan verified by unit tests

---
*Phase: 02-cli-k8s-integration*
*Completed: 2026-03-16*
