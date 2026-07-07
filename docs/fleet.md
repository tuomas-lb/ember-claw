# Fleet Guide — running bots in Kubernetes, mTLS-protected, with a web dashboard

This is the canonical, reproducible way to run one or more PicoClaw bots on a
Kubernetes cluster: each bot as a deployment, a shared web **dashboard** control
plane, protected by **mTLS** client certificates, with **persistent chat
history**. It replaces the earlier hand-rolled setup — every step below is an
`eclaw` command, so a human operator *or* a fleet-admin bot can run it.

## Concepts

| Piece | What it is |
|-------|------------|
| **Instance** | One bot = a `picoclaw-<name>` Deployment + Service (gRPC :50051, HTTP :8080) + PVC + config Secret, created by `eclaw deploy`. |
| **Dashboard** | A namespace-scoped web control plane (lists/deploys/deletes instances, chat, logs, config, fleet registry). One per namespace. `eclaw dashboard deploy`. |
| **mTLS** | nginx client-certificate auth on the ingress. `eclaw mtls init` makes the CA + a `client.p12` you import into your browser. |
| **Shared storage** | A fleet-wide PVC (`--shared-pvc`) mounted into every bot at `/home/picoclaw/shared` for file exchange. Survives instance deletion. |
| **Fleet-admin** | `--fleet-admin` gives a bot a namespace-scoped ServiceAccount + the `eclaw` binary, so it can manage its siblings. |

**Namespace = trust boundary.** Everything is namespace-scoped. Put a bot (or a
group that should see each other) in its own namespace. A fleet-admin bot and the
dashboard can only touch their own namespace, including reading instance secrets —
so don't co-locate unrelated tenants.

## One-time cluster prerequisites

- An nginx ingress controller and cert-manager with a ClusterIssuer (e.g. `letsencrypt-prod`).
- A container registry reachable by the cluster; `IMAGE_REGISTRY` set in `.env`.
- Build + push the images once: `make build-push-picoclaw` and `make build-push-dashboard` (set `EMBER_VERSION=x.y` for versioned tags).
- If your registry needs auth: `eclaw set-registry <registry> --username ... --password ...` in each namespace you deploy to.

## Provision a namespace from scratch

```bash
NS=myfleet
# 1. Deploy the first bot (creates the namespace).
eclaw deploy alice --namespace $NS \
  --provider openrouter --model deepseek/deepseek-v4-pro \
  --shared-pvc myfleet-shared --fleet-admin --playwright \
  --github-token "$GITHUB_TOKEN"
eclaw set-telegram alice --namespace $NS --token "$TG" --allow-from <your-id>

# 2. Generate mTLS material (CA + your browser cert).
eclaw mtls init --client "$(whoami)" --out ./myfleet-mtls

# 3. Deploy the dashboard, protected by that CA, with chat persistence.
eclaw dashboard deploy --namespace $NS \
  --host fleet.example.com \
  --issuer letsencrypt-prod \
  --mtls-ca ./myfleet-mtls/ca.crt \
  --with-postgres

# 4. Point DNS (fleet.example.com) at the ingress load-balancer IP, wait for the
#    cert to issue, then import ./myfleet-mtls/client.p12 into your browser.
```

Open `https://fleet.example.com`, select the cert when prompted — you get the
fleet dashboard. Deploy/manage more bots from the UI or the CLI.

> **Security:** `eclaw dashboard deploy` warns and proceeds without `--mtls-ca`,
> but the dashboard exposes deploy/delete/secret operations — never expose it
> publicly without mTLS (or an equivalent auth layer in front).

## A bot spinning up more bots (self-replication)

A bot deployed with `--fleet-admin` has `eclaw` on its PATH, authenticated via its
in-pod ServiceAccount (no kubeconfig needed), with `ECLAW_NAMESPACE`/`ECLAW_IMAGE`
pre-set. From inside the bot (via chat or exec):

```bash
eclaw list
eclaw deploy worker-1 --provider openrouter --model deepseek/deepseek-v4-pro \
  --shared-pvc myfleet-shared --fleet-admin
eclaw logs worker-1 --tail 50
eclaw chat worker-1 -m "status?"
eclaw delete worker-1
```

The bot can only act within its own namespace, and Kubernetes RBAC prevents it
from granting privileges it doesn't already have.

## Exposing other interfaces (e.g. a backlog board)

Any HTTP port can be mTLS-fronted the same way — reuse the CA from `mtls init`:

```bash
eclaw expose <instance> --namespace $NS --type ingress \
  --host board.example.com --tls --issuer letsencrypt-prod \
  --mtls-ca ./myfleet-mtls/ca.crt
```

## Reference

| Command | Purpose |
|---------|---------|
| `eclaw mtls init` | Generate CA + client.p12 for client-cert auth |
| `eclaw dashboard deploy` | Deploy the fleet dashboard (RBAC, ingress, mTLS, optional Postgres) |
| `eclaw dashboard delete` | Remove the dashboard (Postgres PVC retained) |
| `eclaw deploy --fleet-admin` | Grant a bot rights to manage siblings |
| `eclaw deploy --shared-pvc` | Fleet-wide shared storage |
| `eclaw expose --mtls-ca` | mTLS-protect an instance's own port |
