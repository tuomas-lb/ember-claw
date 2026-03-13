# Research Summary: Ember-Claw

**Domain:** Kubernetes deployment toolkit with gRPC sidecar for AI assistant fleet management
**Researched:** 2026-03-13
**Overall confidence:** HIGH

## Executive Summary

Ember-Claw is a deployment toolkit for running multiple PicoClaw AI assistant instances on a Kubernetes cluster. After analyzing PicoClaw's source code in depth, the architecture is clear: a gRPC sidecar binary that imports PicoClaw as a Go library (using its well-designed `AgentLoop.ProcessDirect()` API), paired with a Go CLI tool that manages instance lifecycle via `client-go`.

The key architectural insight is that PicoClaw should NOT be wrapped as a subprocess. Its interactive CLI mode uses `readline` with terminal escape codes, making stdin/stdout parsing fragile. Instead, the sidecar imports PicoClaw's `pkg/agent` package directly and calls `ProcessDirect(ctx, message, sessionKey)`, which returns a clean string response. This is the same API PicoClaw's own gateway mode uses internally.

The project fits neatly into three build phases: (1) Proto definitions + sidecar binary with PicoClaw integration, (2) CLI tool with K8s management and gRPC client, (3) Makefile and deployment automation. Each phase produces a working, testable artifact.

The technology choices are straightforward since the constraints (Go, Kubernetes, gRPC) are already decided. The main decisions are around project structure, K8s resource patterns, and how the CLI communicates with pods (port-forward, not Ingress).

## Key Findings

**Stack:** Go monorepo with two binaries (sidecar + CLI), protobuf for gRPC contract, client-go for K8s API, docker buildx for images
**Architecture:** Single-container pods importing PicoClaw as library, CLI communicates via port-forward gRPC, one PVC per instance
**Critical pitfall:** Do not wrap PicoClaw as a subprocess -- use its Go library API directly

## Implications for Roadmap

Based on research, suggested phase structure:

1. **Proto + Sidecar** - Build the core gRPC server that wraps PicoClaw
   - Addresses: gRPC sidecar, chat protocol, single-shot query
   - Avoids: Subprocess wrapping pitfall
   - Deliverable: Sidecar binary that can run locally and respond to gRPC calls

2. **CLI + K8s Integration** - Build the management tool
   - Addresses: deploy, list, delete, status, logs, chat, query commands
   - Avoids: kubectl shelling pitfall
   - Deliverable: `eclaw` binary that manages instances on the cluster

3. **Deployment Automation** - Makefile, Dockerfile, templates
   - Addresses: Interactive deployment, build/push pipeline, manifest templates
   - Avoids: Helm overengineering
   - Deliverable: Complete workflow from `make deploy-picoclaw` to chatting

**Phase ordering rationale:**
- Proto must come first because both sidecar and CLI depend on generated gRPC code
- Sidecar must be buildable before CLI can have anything to connect to
- CLI needs working sidecar for integration testing of chat/query
- Makefile/deployment is the final layer that wires everything for production use

**Research flags for phases:**
- Phase 1: May need research into PicoClaw's config loading when used as a library (config path resolution)
- Phase 2: Standard client-go patterns, unlikely to need research
- Phase 3: Standard Docker/Make patterns matching parent repo, unlikely to need research

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Constraints predetermine most choices (Go, gRPC, K8s) |
| Features | HIGH | PROJECT.md requirements are clear and well-scoped |
| Architecture | HIGH | Based on direct PicoClaw source analysis, not speculation |
| Pitfalls | HIGH | Subprocess wrapping pitfall identified from code analysis |

## Gaps to Address

- PicoClaw's config file path resolution when imported as library (it uses `~/.picoclaw/` by default -- need to override for container paths)
- Whether PicoClaw's `go.mod` dependencies are compatible with client-go (potential version conflicts in large dependency trees)
- Exact container resource requirements for PicoClaw (stated <10MB RAM, but with LLM provider SDKs loaded it may be higher)
- Session key isolation strategy -- how to namespace sessions per gRPC client connection
