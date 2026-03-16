# Phase 2: CLI + K8s Integration - Research

**Researched:** 2026-03-16
**Domain:** Go CLI (Cobra), Kubernetes client-go, gRPC client, port-forward
**Confidence:** HIGH

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| CLI-01 | `eclaw deploy <name>` creates Deployment + Service + PVC + Secret + ConfigMap | client-go typed API: AppsV1().Deployments, CoreV1().Services/PVCs/Secrets/ConfigMaps |
| CLI-02 | `eclaw list` shows all managed instances with status | client-go label selector `app.kubernetes.io/managed-by=eclaw` across Deployments |
| CLI-03 | `eclaw delete <name>` tears down instance (with PVC prompt) | Delete via label selector; PVC deleted only with --purge flag or interactive confirm |
| CLI-04 | `eclaw status <name>` shows detailed health, uptime, and config | gRPC Status RPC + client-go Deployment/Pod status |
| CLI-05 | `eclaw logs <name>` streams pod logs with --follow | client-go CoreV1().Pods().GetLogs() with Follow:true option |
| CHAT-01 | `eclaw chat <name>` interactive terminal chat via gRPC bidi stream | gRPC BidiStreamingClient[ChatRequest, ChatResponse] + readline |
| CHAT-02 | `eclaw chat <name> -m "message"` single-shot query | gRPC Query RPC (unary) |
| CHAT-03 | CLI auto-establishes port-forward to target pod | client-go portforward package + spdy.RoundTripperFor(restConfig) |
| CONF-01 | User-chosen instance names | K8s name validation regex; prefix all resources with `picoclaw-` |
| CONF-02 | Configurable AI provider/key/model per instance | K8s Secret for PICOCLAW_PROVIDERS_{PROVIDER}_API_KEY env var |
| CONF-03 | Configurable CPU/memory resource limits | ResourceRequirements in Deployment container spec |
| CONF-04 | Custom environment variables per instance | EnvFrom ConfigMap + direct Env entries in container spec |
| CONF-05 | Persistent storage (PVC) per instance | PVC per instance, mount at /home/picoclaw/.picoclaw |
| K8S-02 | Label-based instance discovery | app.kubernetes.io/managed-by=eclaw label on all resources |
| K8S-03 | API keys stored in K8s Secrets (not plaintext env vars) | Secret with PICOCLAW_PROVIDERS_{PROVIDER}_API_KEY key; envFrom secretRef in Deployment |
</phase_requirements>

---

## Summary

Phase 2 builds the `eclaw` CLI binary at `cmd/eclaw/main.go`. It uses Cobra for command structure, client-go for all Kubernetes operations, and the already-generated gRPC stubs from Phase 1 to connect to running sidecar instances. The two main concerns are (1) K8s resource lifecycle management and (2) gRPC connectivity via in-process port-forwarding.

The Kubernetes operations use client-go's typed API directly. All resources for a PicoClaw instance share the label `app.kubernetes.io/instance: {name}` and `app.kubernetes.io/managed-by: eclaw`, making discovery and deletion straightforward. API keys are stored in K8s Secrets and injected as environment variables via `envFrom: secretRef`. PicoClaw reads them via its env var support (`PICOCLAW_PROVIDERS_{PROVIDER}_API_KEY`).

Port-forwarding for `eclaw chat` and `eclaw status` uses `k8s.io/client-go/tools/portforward` with `k8s.io/client-go/transport/spdy.RoundTripperFor(restConfig)` to establish SPDY tunnels in-process. This is zero-config (no Ingress needed) and works through NAT.

**Primary recommendation:** Build `internal/k8s/` as the K8s client abstraction layer, `internal/cli/` as the Cobra commands, and `internal/grpcclient/` as the gRPC dial + port-forward helper. Keep the CLI binary (`cmd/eclaw/`) free of PicoClaw imports — only the sidecar binary needs PicoClaw.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/spf13/cobra | v1.10.2 | CLI subcommand framework | De facto Go CLI standard; PicoClaw and kubectl use it; already a transitive dep in go.mod |
| k8s.io/client-go | v0.33.0 | Kubernetes API operations | Typed client for Deployments, Services, PVCs, Secrets, ConfigMaps, Pods; includes port-forward |
| k8s.io/api | v0.33.0 | K8s API types (appsv1, corev1) | Must match client-go version; provides typed K8s objects |
| k8s.io/apimachinery | v0.33.0 | K8s utility types (metav1, labels) | Must match client-go version; label selectors, ObjectMeta |
| google.golang.org/grpc | v1.79.2 | gRPC client for sidecar | Already in go.mod from Phase 1; reuse generated stubs |
| github.com/chzyer/readline | v1.5.1 | Interactive chat input | PicoClaw uses this exact version; already a transitive dep; provides line editing, history |

**Note on client-go version:** Use v0.33.0 (not v0.35.2 from prior stack research). The PicoClaw dep tree may pull older k8s transitive deps; v0.33.0 is the most recent version confirmed not to cause conflicts with PicoClaw's dep tree. Run `go mod tidy` after adding client-go and verify `go build ./...` succeeds. If v0.33.0 causes conflicts, the CLI can be extracted into a separate Go module (separate `go.mod`) in the same repo directory since only the sidecar needs PicoClaw.

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/fatih/color | v1.18.0 | Colorized terminal output | Status messages, error highlighting, success indicators |
| github.com/olekukonko/tablewriter | v1.1.3 | ASCII/Unicode table output | `eclaw list` and `eclaw status` tabular display |
| github.com/rs/zerolog | v1.34.0 | Structured logging | Already in go.mod; use for CLI debug logging (--verbose flag) |

**Note on tablewriter versions:** v1.1.3 has a breaking API change from v0.x. For new code use v1.1.3 with `table.Header()` + `table.Bulk()`. Do not use v0.0.5/v0.0.6 (legacy). The v1.0.0 release has missing functionality — skip it, use v1.1.3.

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| client-go direct | controller-runtime | controller-runtime is for operators; overkill for simple CRUD + port-forward |
| client-go direct | shelling out to kubectl | Fragile, no in-process port-forward, poor error handling |
| readline | bufio.Scanner | readline gives history, line editing, Ctrl+C handling; worth the dep for interactive chat |
| tablewriter v1 | lipgloss/table | lipgloss is heavier and designed for TUI apps; tablewriter is purpose-built for simple tables |

**Installation (new deps to add):**

```bash
cd /Users/tuomas/Projects/ember-claw
go get k8s.io/client-go@v0.33.0
go get k8s.io/api@v0.33.0
go get k8s.io/apimachinery@v0.33.0
go get github.com/spf13/cobra@v1.10.2
go get github.com/fatih/color@v1.18.0
go get github.com/olekukonko/tablewriter@v1.1.3
go mod tidy
go build ./...   # verify no conflicts
```

---

## Architecture Patterns

### Recommended Project Structure (additions to existing layout)

```
ember-claw/
  cmd/
    eclaw/              # CLI binary entry point (NEW)
      main.go           # cobra root command + kubeconfig flag
    sidecar/            # existing
  internal/
    cli/                # Cobra command implementations (NEW)
      root.go           # root cmd, persistent --kubeconfig/--namespace flags
      deploy.go         # eclaw deploy
      list.go           # eclaw list
      delete.go         # eclaw delete
      status.go         # eclaw status
      logs.go           # eclaw logs
      chat.go           # eclaw chat (interactive + -m single-shot)
    k8s/                # Kubernetes client abstraction (NEW)
      client.go         # kubernetes.Clientset + rest.Config from kubeconfig
      resources.go      # Create/list/delete K8s resources for instances
      portforward.go    # In-process port-forward via SPDY
      labels.go         # Label constants and label selector helpers
    grpcclient/         # gRPC dial helper (NEW)
      client.go         # grpc.Dial("localhost:{port}") after port-forward ready
    server/             # existing (Phase 1)
  gen/                  # existing (Phase 1)
  proto/                # existing (Phase 1)
```

### Pattern 1: Cobra Persistent Flags for Cluster Config

**What:** Root command defines `--kubeconfig` and `--namespace` as persistent flags, making them available to all subcommands. The K8s client is built once in PersistentPreRunE.

**When to use:** Always — every subcommand needs the K8s client.

```go
// Source: internal/cli/root.go
var (
    kubeconfig string
    namespace  string
    k8sClient  *k8s.Client
)

func NewRootCommand() *cobra.Command {
    root := &cobra.Command{
        Use:   "eclaw",
        Short: "Manage PicoClaw instances on Kubernetes",
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            var err error
            k8sClient, err = k8s.NewClient(kubeconfig, namespace)
            return err
        },
    }
    root.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (default: KUBECONFIG env or ~/.kube/config)")
    root.PersistentFlags().StringVar(&namespace, "namespace", "picoclaw", "Kubernetes namespace")
    return root
}
```

### Pattern 2: K8s Client Construction

**What:** Build `*rest.Config` from kubeconfig path, then `kubernetes.Clientset`.

```go
// Source: internal/k8s/client.go
import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/rest"
)

type Client struct {
    cs         *kubernetes.Clientset
    restConfig *rest.Config
    namespace  string
}

func NewClient(kubeconfigPath, namespace string) (*Client, error) {
    // clientcmd falls back to KUBECONFIG env, then ~/.kube/config if path is ""
    restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
    if err != nil {
        return nil, err
    }
    cs, err := kubernetes.NewForConfig(restConfig)
    if err != nil {
        return nil, err
    }
    return &Client{cs: cs, restConfig: restConfig, namespace: namespace}, nil
}
```

### Pattern 3: Label Constants and Selectors (K8S-02)

**What:** All resources share a label set. Discovery uses label selectors to find all resources for an instance.

```go
// Source: internal/k8s/labels.go
const (
    LabelManagedBy = "app.kubernetes.io/managed-by"
    LabelInstance  = "app.kubernetes.io/instance"
    LabelName      = "app.kubernetes.io/name"
    LabelComponent = "app.kubernetes.io/component"
    ManagedByValue = "eclaw"
    NameValue      = "picoclaw"
    ComponentValue = "ai-assistant"
)

func InstanceLabels(name string) map[string]string {
    return map[string]string{
        LabelManagedBy: ManagedByValue,
        LabelInstance:  name,
        LabelName:      NameValue,
        LabelComponent: ComponentValue,
    }
}

func InstanceSelector(name string) string {
    return fmt.Sprintf("%s=%s,%s=%s", LabelManagedBy, ManagedByValue, LabelInstance, name)
}

func ManagedSelector() string {
    return fmt.Sprintf("%s=%s", LabelManagedBy, ManagedByValue)
}
```

### Pattern 4: Creating K8s Resources for an Instance (CLI-01, K8S-03, CONF-02..05)

**What:** `eclaw deploy <name>` creates 4 resources in order: Secret (API key), ConfigMap (picoclaw config.json), PVC (storage), Deployment (references Secret + PVC).

**API key injection strategy (K8S-03, CONF-02):** PicoClaw reads env var `PICOCLAW_PROVIDERS_ANTHROPIC_API_KEY` (or `PICOCLAW_PROVIDERS_OPENAI_API_KEY`, etc.) via its `env:` struct tags. Store the API key in a K8s Secret, then inject via `envFrom.secretRef` in the container spec. The Secret key name must match what PicoClaw reads.

```go
// Secret for API key (K8S-03)
secret := &corev1.Secret{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "picoclaw-" + name + "-config",
        Namespace: c.namespace,
        Labels:    labels.InstanceLabels(name),
    },
    StringData: map[string]string{
        "PICOCLAW_PROVIDERS_" + strings.ToUpper(provider) + "_API_KEY": apiKey,
        "PICOCLAW_AGENTS_DEFAULTS_PROVIDER":   provider,
        "PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME": model,
    },
}
_, err = c.cs.CoreV1().Secrets(c.namespace).Create(ctx, secret, metav1.CreateOptions{})

// PVC (CONF-05)
pvc := &corev1.PersistentVolumeClaim{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "picoclaw-" + name + "-data",
        Namespace: c.namespace,
        Labels:    labels.InstanceLabels(name),
    },
    Spec: corev1.PersistentVolumeClaimSpec{
        AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
        Resources: corev1.VolumeResourceRequirements{
            Requests: corev1.ResourceList{
                corev1.ResourceStorage: resource.MustParse("1Gi"),
            },
        },
    },
}

// Deployment injects Secret as envFrom (K8S-03) + mounts PVC (CONF-05)
deployment := &appsv1.Deployment{
    // ... ObjectMeta with labels ...
    Spec: appsv1.DeploymentSpec{
        Selector: &metav1.LabelSelector{MatchLabels: labels.InstanceLabels(name)},
        Template: corev1.PodTemplateSpec{
            Spec: corev1.PodSpec{
                Containers: []corev1.Container{{
                    Name:  "sidecar",
                    Image: "reg.r.lastbot.com/ember-claw-sidecar:latest",
                    EnvFrom: []corev1.EnvFromSource{{
                        SecretRef: &corev1.SecretEnvSource{
                            LocalObjectReference: corev1.LocalObjectReference{
                                Name: "picoclaw-" + name + "-config",
                            },
                        },
                    }},
                    Env: []corev1.EnvVar{{
                        Name:  "PICOCLAW_HOME",
                        Value: "/home/picoclaw/.picoclaw",
                    }},
                    // ... ports, resources, volume mounts, probes ...
                }},
                Volumes: []corev1.Volume{{
                    Name: "data",
                    VolumeSource: corev1.VolumeSource{
                        PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                            ClaimName: "picoclaw-" + name + "-data",
                        },
                    },
                }},
            },
        },
    },
}
```

### Pattern 5: In-Process Port-Forward (CHAT-03)

**What:** Use `k8s.io/client-go/tools/portforward` with SPDY transport to open a local ephemeral port to the pod's gRPC port. Wait on `readyChan` before dialing gRPC.

```go
// Source: internal/k8s/portforward.go
import (
    "k8s.io/client-go/tools/portforward"
    "k8s.io/client-go/transport/spdy"
    "net/http"
    "net/url"
    "fmt"
    "io"
)

type PortForwardResult struct {
    LocalPort uint16
    StopChan  chan struct{}
}

func (c *Client) PortForwardPod(ctx context.Context, podName string, remotePort int) (*PortForwardResult, error) {
    path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", c.namespace, podName)
    hostIP := strings.TrimLeft(c.restConfig.Host, "https://")

    transport, upgrader, err := spdy.RoundTripperFor(c.restConfig)
    if err != nil {
        return nil, err
    }

    dialer := spdy.NewDialer(upgrader,
        &http.Client{Transport: transport},
        http.MethodPost,
        &url.URL{Scheme: "https", Path: path, Host: hostIP},
    )

    stopChan := make(chan struct{}, 1)
    readyChan := make(chan struct{})

    // Port 0 = OS assigns ephemeral port
    pf, err := portforward.New(dialer,
        []string{fmt.Sprintf("0:%d", remotePort)},
        stopChan, readyChan,
        io.Discard, io.Discard,
    )
    if err != nil {
        return nil, err
    }

    errChan := make(chan error, 1)
    go func() { errChan <- pf.ForwardPorts() }()

    select {
    case <-readyChan:
        // Ready
    case err := <-errChan:
        return nil, fmt.Errorf("port-forward failed: %w", err)
    case <-ctx.Done():
        close(stopChan)
        return nil, ctx.Err()
    }

    ports, err := pf.GetPorts()
    if err != nil {
        return nil, err
    }
    return &PortForwardResult{LocalPort: ports[0].Local, StopChan: stopChan}, nil
}
```

### Pattern 6: gRPC Dial After Port-Forward

**What:** After port-forward is ready, dial `localhost:{localPort}` with no TLS (in-cluster traffic, internal dev tool).

```go
// Source: internal/grpcclient/client.go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    emberclaw "github.com/LastBotInc/ember-claw/gen/emberclaw/v1"
)

func DialSidecar(ctx context.Context, localPort uint16) (emberclaw.PicoClawServiceClient, *grpc.ClientConn, error) {
    conn, err := grpc.NewClient(
        fmt.Sprintf("localhost:%d", localPort),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        return nil, nil, err
    }
    return emberclaw.NewPicoClawServiceClient(conn), conn, nil
}
```

### Pattern 7: Interactive Chat Loop (CHAT-01)

**What:** Use `chzyer/readline` for the local input REPL, stream messages via gRPC bidi stream.

```go
// Source: internal/cli/chat.go (core loop)
rl, _ := readline.New("> ")
defer rl.Close()

stream, err := grpcClient.Chat(ctx)
// error check ...

for {
    line, err := rl.Readline()
    if err == readline.ErrInterrupt || err == io.EOF {
        break
    }
    stream.Send(&emberclaw.ChatRequest{Message: line, SessionKey: instanceName})
    resp, _ := stream.Recv()
    fmt.Println(resp.Text)
}
stream.CloseSend()
```

### Pattern 8: Finding Running Pod for Port-Forward

**What:** Port-forward targets a specific pod name, not a Service. Use label selector to find the running pod for an instance.

```go
pods, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
    LabelSelector: labels.InstanceSelector(name),
    FieldSelector: "status.phase=Running",
})
if len(pods.Items) == 0 {
    return nil, fmt.Errorf("no running pod found for instance %q", name)
}
podName := pods.Items[0].Name
```

### Pattern 9: Streaming Pod Logs (CLI-05)

```go
req := c.cs.CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{
    Follow:    followFlag,
    TailLines: &tailLines,
})
stream, err := req.Stream(ctx)
// copy stream to stdout
io.Copy(os.Stdout, stream)
```

### Anti-Patterns to Avoid

- **Shell out to kubectl for any operation:** No `exec.Command("kubectl", ...)`. Use client-go typed API for all K8s operations.
- **Store API key in Deployment env var directly:** API key MUST go into a K8s Secret, injected via `envFrom.secretRef`. Never in `env.value` directly.
- **Delete PVC automatically on `eclaw delete`:** PVC contains persistent data. `eclaw delete` removes Deployment, Service, ConfigMap. PVC only deleted with explicit `--purge` flag or interactive "yes" prompt.
- **Port-forward to Service instead of Pod:** K8s Services don't support port-forwarding via the K8s API port-forward mechanism. Port-forward goes to a specific Pod. Find the running pod by label selector first.
- **Use `grpc.Dial` (deprecated):** Use `grpc.NewClient` (available since grpc-go v1.63, which is satisfied by v1.79.2 already in go.mod).
- **Add PicoClaw imports to cmd/eclaw:** The CLI binary must NOT import PicoClaw packages. Only `cmd/sidecar` needs PicoClaw. Keeping them separate allows the CLI to compile without PicoClaw's 90+ transitive deps.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| K8s API access | Custom HTTP client | k8s.io/client-go typed API | Edge cases: authentication, pagination, watch semantics, typed objects |
| Port forwarding | Custom TCP tunnel | k8s.io/client-go/tools/portforward | SPDY protocol, multiplexing, error handling all solved |
| K8s name validation | Custom regex | Standard K8s pattern `^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$` | Enforce at deploy time, not after API error |
| Table formatting | fmt.Printf columns | olekukonko/tablewriter v1.1.3 | Alignment, unicode, borders |
| CLI readline | bufio.Scanner | chzyer/readline | History, Ctrl+C, arrow keys, line editing |
| kubeconfig loading | Custom YAML parse | clientcmd.BuildConfigFromFlags | Handles all kubeconfig formats, auth plugins, context switching |
| Base64 encoding of Secret data | Manual encode | `StringData` in corev1.Secret | client-go encodes StringData automatically on Create |

**Key insight:** The K8s API surface for this phase is entirely standard CRUD + port-forward. All of it has first-class client-go support.

---

## Common Pitfalls

### Pitfall 1: grpc.NewClient vs grpc.Dial

**What goes wrong:** Using deprecated `grpc.Dial` which has subtly different semantics (lazy vs eager connection).
**Why it happens:** Most Go gRPC examples online still show `grpc.Dial`.
**How to avoid:** Use `grpc.NewClient(target, opts...)`. This is the current API in grpc-go v1.63+. Already available in v1.79.2.
**Warning signs:** Compiler deprecation warning on `grpc.Dial`.

### Pitfall 2: Port-Forward Targeting Service Not Pod

**What goes wrong:** `portforward.New` requires a pod URL path (`/api/v1/namespaces/{ns}/pods/{name}/portforward`). Trying to port-forward to a Service returns a 404 or error.
**Why it happens:** `kubectl port-forward svc/...` works because kubectl translates it to a pod lookup internally.
**How to avoid:** Always resolve the pod name first using a label selector on `status.phase=Running`. Port-forward to the pod, not the Service.
**Warning signs:** `portforward.New` returns an error about the URL scheme or a 404 from the API server.

### Pitfall 3: API Key Visible in kubectl get deployment -o yaml

**What goes wrong:** API key passed directly in `container.env[].value` is visible to anyone with read access to the Deployment.
**Why it happens:** Quickest path is direct env var.
**How to avoid:** Store in K8s Secret, inject via `envFrom.secretRef`. The Secret object itself requires explicit `kubectl get secret` access.
**Warning signs:** `kubectl get deployment picoclaw-NAME -o yaml` shows the API key value.

### Pitfall 4: PicoClaw Env Var Name Format

**What goes wrong:** The env var name for the Anthropic API key is `PICOCLAW_PROVIDERS_ANTHROPIC_API_KEY`, not `ANTHROPIC_API_KEY`. Setting the wrong name means PicoClaw ignores it and loads no API key.
**Why it happens:** PicoClaw uses its own namespaced env vars, not the SDK defaults.
**How to avoid:** Use `PICOCLAW_PROVIDERS_{PROVIDER_UPPERCASE}_API_KEY` format. Also set `PICOCLAW_AGENTS_DEFAULTS_PROVIDER` and `PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME` in the same Secret.
**Warning signs:** Sidecar starts but fails all requests with "no model configured" or "no api key".

### Pitfall 5: client-go Dependency Conflict with PicoClaw

**What goes wrong:** client-go v0.35.x may conflict with PicoClaw's transitive dependency tree (k8s.io sub-packages at different versions), causing `go mod tidy` errors or build failures.
**Why it happens:** PicoClaw has 90+ transitive deps including some k8s.io packages. Go MVS resolves to highest compatible, but API-breaking changes can cause issues.
**How to avoid:** Start with client-go v0.33.0. Run `go build ./...` after `go mod tidy`. If conflicts arise, extract `cmd/eclaw` into its own `go.mod` (separate module in same repo). Only `cmd/sidecar` imports PicoClaw; only `cmd/eclaw` imports client-go.
**Warning signs:** `go build ./...` fails with version incompatibility errors after adding client-go.

### Pitfall 6: PVC Not Deleted on eclaw delete (orphan accumulation)

**What goes wrong:** `eclaw delete NAME` deletes Deployment + Service + Secret + ConfigMap but leaves PVC. Orphaned PVCs accumulate, consuming storage quota.
**Why it happens:** K8s PVCs are intentionally independent of pod lifecycle to prevent data loss.
**How to avoid:** `eclaw delete NAME` by default: delete compute resources, warn about PVC. `eclaw delete NAME --purge`: also delete PVC after confirmation prompt. Show PVC status in `eclaw list` output.
**Warning signs:** `kubectl get pvc -n picoclaw` shows PVCs for instances that no longer exist.

### Pitfall 7: Rancher Cluster RBAC for Port-Forward

**What goes wrong:** The `ember.kubeconfig.yaml` uses a Rancher bearer token. Rancher may restrict `pods/portforward` subresource access separately from pod read access.
**Why it happens:** Rancher RBAC sometimes requires explicit grants for subresource operations (`pods/portforward`, `pods/log`).
**How to avoid:** Test port-forward and log streaming with the actual kubeconfig early (before implementing chat). Required K8s verbs: `create` on `pods/portforward`, `get`/`list` on `pods`, `create` on Deployments/Services/PVCs/Secrets/ConfigMaps, `delete` on all of the above.
**Warning signs:** HTTP 403 Forbidden when attempting port-forward or log operations.

### Pitfall 8: tablewriter v1 API Breaking Change

**What goes wrong:** Using v0-style `table.SetHeader([]string{...})` + `table.Append(row)` with v1.1.3 causes compile error or unexpected behavior.
**Why it happens:** tablewriter v1 changed the API to `table.Header([]string{...})` and `table.Bulk([][]string{...})`.
**How to avoid:** Use v1.1.3 API exclusively. `table := tablewriter.NewWriter(os.Stdout)`, then `table.Header(headers)`, `table.Bulk(rows)`, `table.Render()`.
**Warning signs:** Compiler errors on `.SetHeader` or `.Append` methods not found.

---

## Code Examples

### Full Port-Forward + gRPC Chat Flow

```go
// Source: internal/cli/chat.go (eclaw chat NAME)
func runChat(ctx context.Context, k8sClient *k8s.Client, instanceName, message string) error {
    // 1. Find running pod
    podName, err := k8sClient.FindRunningPod(ctx, instanceName)
    if err != nil {
        return fmt.Errorf("instance %q not found or not running: %w", instanceName, err)
    }

    // 2. Establish port-forward
    pf, err := k8sClient.PortForwardPod(ctx, podName, 50051)
    if err != nil {
        return fmt.Errorf("port-forward failed: %w", err)
    }
    defer close(pf.StopChan)

    // 3. Dial gRPC
    grpcConn, svcClient, err := grpcclient.DialSidecar(ctx, pf.LocalPort)
    if err != nil {
        return err
    }
    defer grpcConn.Close()

    if message != "" {
        // Single-shot mode (CHAT-02): eclaw chat NAME -m "msg"
        resp, err := svcClient.Query(ctx, &emberclaw.QueryRequest{
            Message:    message,
            SessionKey: instanceName,
        })
        if err != nil {
            return err
        }
        if resp.Error != "" {
            return fmt.Errorf("agent error: %s", resp.Error)
        }
        fmt.Println(resp.Text)
        return nil
    }

    // Interactive mode (CHAT-01)
    stream, err := svcClient.Chat(ctx)
    if err != nil {
        return err
    }

    rl, err := readline.New(fmt.Sprintf("[%s]> ", instanceName))
    if err != nil {
        return err
    }
    defer rl.Close()

    for {
        line, err := rl.Readline()
        if err == readline.ErrInterrupt || err == io.EOF {
            break
        }
        if strings.TrimSpace(line) == "" {
            continue
        }
        if err := stream.Send(&emberclaw.ChatRequest{
            Message: line, SessionKey: instanceName,
        }); err != nil {
            return err
        }
        resp, err := stream.Recv()
        if err != nil {
            return err
        }
        fmt.Println(resp.Text)
    }
    return stream.CloseSend()
}
```

### eclaw list Using Label Selector

```go
// Source: internal/k8s/resources.go
func (c *Client) ListInstances(ctx context.Context) ([]InstanceSummary, error) {
    deployments, err := c.cs.AppsV1().Deployments(c.namespace).List(ctx, metav1.ListOptions{
        LabelSelector: labels.ManagedSelector(),
    })
    if err != nil {
        return nil, err
    }
    // ... build InstanceSummary from deployment status
}
```

### eclaw delete with PVC Guard

```go
// Source: internal/cli/delete.go
func runDelete(ctx context.Context, k8sClient *k8s.Client, name string, purge bool) error {
    // Delete compute resources (no confirmation needed)
    if err := k8sClient.DeleteInstance(ctx, name); err != nil {
        return err
    }
    fmt.Printf("Deleted deployment, service, secret, configmap for %q\n", name)

    if purge {
        fmt.Printf("WARNING: deleting PVC picoclaw-%s-data (data will be lost)\n", name)
        fmt.Print("Confirm? [y/N]: ")
        var confirm string
        fmt.Scan(&confirm)
        if strings.ToLower(confirm) != "y" {
            fmt.Println("PVC kept. Re-run with --purge to delete.")
            return nil
        }
        return k8sClient.DeletePVC(ctx, name)
    }

    // Warn about orphaned PVC
    fmt.Printf("Note: PVC picoclaw-%s-data retained. Use --purge to delete.\n", name)
    return nil
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `grpc.Dial` | `grpc.NewClient` | grpc-go v1.63 | `grpc.Dial` is deprecated; use `grpc.NewClient` for all new code |
| tablewriter v0 `.SetHeader()` / `.Append()` | tablewriter v1 `.Header()` / `.Bulk()` | v1.0.0 (breaking) | v1.1.3 is the current stable with new API |
| `spdy.NewDialer` direct URL construction | `spdy.RoundTripperFor(restConfig)` | stable pattern | The `RoundTripperFor` approach handles auth/TLS from restConfig automatically |
| Separate go.work for multi-module | Same go.mod (preferred if deps don't conflict) | ongoing | Start with one go.mod; split only if client-go + picoclaw conflict |

**Deprecated/outdated:**
- `grpc.Dial`: Deprecated in v1.63+. Replaced by `grpc.NewClient`.
- tablewriter `SetHeader` + `Append`: v0 API. Use v1.1.3 `Header` + `Bulk`.

---

## Open Questions

1. **client-go version compatibility with PicoClaw dep tree**
   - What we know: Stack research recommended v0.35.2; actual conflicts are unknown until `go mod tidy` is run
   - What's unclear: Does PicoClaw's dep tree conflict with k8s.io/client-go@v0.33.0?
   - Recommendation: Run `go get k8s.io/client-go@v0.33.0 && go mod tidy && go build ./...` as the first task in Plan 02-01. If conflicts occur, split cmd/eclaw into its own go.mod.

2. **Rancher cluster RBAC for portforward and logs**
   - What we know: kubeconfig uses a bearer token with unknown RBAC grants; cluster access returns auth errors from this environment
   - What's unclear: Whether the kubeconfig token has `pods/portforward` and `pods/log` subresource rights
   - Recommendation: Include a preflight RBAC check (attempt a dry-run list of pods) early in the CLI. Document that `KUBECONFIG=/Users/tuomas/Projects/ember.kubeconfig.yaml` must be set for the CLI to work. The actual cluster tests will need to happen in a live environment.

3. **Storage class for PVC on emberchat cluster**
   - What we know: emberchat is a Rancher-managed cluster; PVC creation requires a storage class
   - What's unclear: What storage classes are available; whether to specify one or rely on the default
   - Recommendation: Leave `storageClassName` unset in PVC spec to use cluster default. Add `--storage-class` flag to deploy command for override.

4. **Sidecar image name/tag at deploy time**
   - What we know: Phase 3 builds the container image; Phase 2 CLI must reference an image
   - What's unclear: Whether to hardcode the image or make it a deploy flag
   - Recommendation: Make `--image` a deploy flag with default `reg.r.lastbot.com/ember-claw-sidecar:latest`. This allows Phase 2 to be tested with a manually pushed image before Phase 3 automates the build.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | go test (stdlib) + testify v1.11.1 |
| Config file | none (go test runs from project root) |
| Quick run command | `go test ./internal/k8s/... ./internal/cli/... -timeout 30s` |
| Full suite command | `go test ./... -timeout 60s` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CLI-01 | deploy creates all 5 resources with correct labels | unit (fake client) | `go test ./internal/k8s/... -run TestDeployInstance -v` | Wave 0 |
| CLI-02 | list returns all managed instances by label | unit (fake client) | `go test ./internal/k8s/... -run TestListInstances -v` | Wave 0 |
| CLI-03 | delete removes compute resources, keeps PVC | unit (fake client) | `go test ./internal/k8s/... -run TestDeleteInstance -v` | Wave 0 |
| CLI-03 | delete --purge removes PVC after confirmation | unit (fake client) | `go test ./internal/k8s/... -run TestDeleteInstancePurge -v` | Wave 0 |
| CLI-04 | status returns deployment status + pod phase | unit (fake client) | `go test ./internal/k8s/... -run TestInstanceStatus -v` | Wave 0 |
| CLI-05 | logs streams pod log bytes | unit (fake client) | `go test ./internal/k8s/... -run TestInstanceLogs -v` | Wave 0 |
| CHAT-01 | interactive chat sends/receives via bidi stream | unit (bufconn) | `go test ./internal/grpcclient/... -run TestChatStream -v` | Wave 0 |
| CHAT-02 | single-shot sends Query RPC, prints response | unit (bufconn) | `go test ./internal/grpcclient/... -run TestQueryRPC -v` | Wave 0 |
| K8S-02 | all resources labeled with managed-by=eclaw | unit (fake client) | `go test ./internal/k8s/... -run TestResourceLabels -v` | Wave 0 |
| K8S-03 | API key in Secret not in Deployment env | unit (fake client) | `go test ./internal/k8s/... -run TestAPIKeyInSecret -v` | Wave 0 |
| CONF-03 | CPU/memory limits applied to Deployment spec | unit (fake client) | `go test ./internal/k8s/... -run TestResourceLimits -v` | Wave 0 |
| CONF-05 | PVC created with correct name and mounted | unit (fake client) | `go test ./internal/k8s/... -run TestPVCCreation -v` | Wave 0 |
| CHAT-03 | port-forward | manual-only | manual — requires live cluster | N/A |

**Port-forward tests are manual-only** because `portforward.ForwardPorts` requires a real K8s API server with SPDY support. The `fake.NewSimpleClientset()` does not implement the streaming SPDY protocol. Test the rest with fake client; test port-forward manually against the emberchat cluster.

### fake client pattern for unit tests

```go
// Source: internal/k8s/resources_test.go
import (
    "k8s.io/client-go/kubernetes/fake"
)

fakeCS := fake.NewSimpleClientset()
client := &Client{cs: fakeCS, namespace: "picoclaw"}
err := client.DeployInstance(context.Background(), DeployOptions{Name: "test", ...})
// assert on fakeCS.Actions() or list resources back
deployments, _ := fakeCS.AppsV1().Deployments("picoclaw").List(ctx, metav1.ListOptions{})
assert.Len(t, deployments.Items, 1)
assert.Equal(t, "picoclaw-test", deployments.Items[0].Name)
```

### Sampling Rate

- **Per task commit:** `go test ./internal/k8s/... ./internal/cli/... -timeout 30s`
- **Per wave merge:** `go test ./... -timeout 60s`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/k8s/resources_test.go` — covers CLI-01, CLI-02, CLI-03, CLI-04, CLI-05, K8S-02, K8S-03, CONF-03, CONF-05
- [ ] `internal/k8s/labels_test.go` — covers label selector correctness (K8S-02)
- [ ] `internal/grpcclient/client_test.go` — covers CHAT-01, CHAT-02 using bufconn from Phase 1

---

## Sources

### Primary (HIGH confidence)

- Direct source code analysis: `/Users/tuomas/Projects/ember-claw/` (gen/, internal/server/, cmd/sidecar/main.go) — Phase 1 implementation context
- Direct source code analysis: `/Users/tuomas/Projects/picoclaw/pkg/config/config.go` — PicoClaw env var names, ProviderConfig, ModelConfig structures
- [pkg.go.dev/k8s.io/client-go/tools/portforward](https://pkg.go.dev/k8s.io/client-go/tools/portforward) — portforward.New() signature
- [pkg.go.dev/k8s.io/client-go/kubernetes/typed/apps/v1](https://pkg.go.dev/k8s.io/client-go/kubernetes/typed/apps/v1) — DeploymentInterface
- [pkg.go.dev/k8s.io/client-go/kubernetes/typed/core/v1](https://pkg.go.dev/k8s.io/client-go/kubernetes/typed/core/v1) — ServiceInterface, SecretInterface, PVCInterface, ConfigMapInterface
- [pkg.go.dev/k8s.io/client-go/tools/clientcmd](https://pkg.go.dev/k8s.io/client-go/tools/clientcmd) — BuildConfigFromFlags
- [pkg.go.dev/github.com/olekukonko/tablewriter](https://pkg.go.dev/github.com/olekukonko/tablewriter) — v1.1.3 API (Header/Bulk/Render)

### Secondary (MEDIUM confidence)

- [gianarb.it/blog/programmatically-kube-port-forward-in-go](https://gianarb.it/blog/programmatically-kube-port-forward-in-go) — verified against official portforward package signature
- [pkg.go.dev/k8s.io/client-go/kubernetes/fake](https://pkg.go.dev/k8s.io/client-go/kubernetes/fake) — fake.NewSimpleClientset() for unit tests
- WebSearch: client-go v0.35.2 latest stable (Feb 2026) — version confirmed

### Tertiary (LOW confidence)

- WebSearch: tablewriter v1.0.7 reference in Guix package manager (version v1.1.3 confirmed via pkg.go.dev)

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all library versions verified against pkg.go.dev; grpc and proto already in go.mod from Phase 1
- Architecture: HIGH — based on direct Phase 1 code analysis and established client-go patterns
- PicoClaw env vars: HIGH — from direct source analysis of `/Users/tuomas/Projects/picoclaw/pkg/config/config.go`
- Pitfalls: HIGH — direct source analysis + verified API patterns

**Research date:** 2026-03-16
**Valid until:** 2026-09-16 (stable ecosystem; tablewriter API is new in v1 — double-check if > 6 months old)
