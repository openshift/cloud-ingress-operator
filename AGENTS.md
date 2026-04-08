# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Operator Does

cloud-ingress-operator toggles OSD/ROSA clusters between "private" and "public" network states. It manages:
- **Default API server** (`api.<cluster-domain>`) visibility (internal/external)
- **Ingress endpoints** (`*.apps.<cluster-domain>`) with optional secondary ingress (`*.apps2`)
- **Admin API endpoint** (`rh-api.<cluster-domain>`) for Hive/SRE access (always available)
- **SSHD endpoint** for cluster debugging access

## Build Commands

All make targets come from the boilerplate system (`boilerplate/openshift/golang-osd-operator/standard.mk`). The root Makefile just includes generated boilerplate targets.

```bash
make                    # Default: go-check → go-test → go-build
make go-build           # Build binary to build/_output/bin/cloud-ingress-operator
make go-test            # Run all unit tests
make go-test TESTOPTS="-v -run TestFunctionName"  # Run specific test verbosely
make go-check           # Run golangci-lint (aliased as `make lint` with YAML validation)
make generate           # Run all code generation (CRDs, deepcopy, openapi)
make validate           # Verify generated/boilerplate code is unmodified and up-to-date
```

Run tests for a single package:
```bash
go test ./pkg/cloudclient/aws/...
go test ./pkg/controller/publishingstrategy/...
```

Container build uses podman (per workspace convention) with a two-stage Dockerfile at `build/Dockerfile`.

## Architecture

### Cloud Provider Abstraction

The `CloudClient` interface (`pkg/cloudclient/cloudclient.go`) defines all cloud operations. Providers register via a factory pattern:

```
pkg/cloudclient/cloudclient.go   — Interface + factory registry
pkg/cloudclient/aws/             — AWS implementation (EC2, ELB/ELBv2, Route53)
pkg/cloudclient/gcp/             — GCP implementation (Compute Engine, Cloud DNS)
pkg/cloudclient/add_aws.go       — Registers AWS factory
pkg/cloudclient/add_gcp.go       — Registers GCP factory
```

Platform is detected at runtime from the cluster's Infrastructure object. `GetClientFor()` panics if no matching provider exists.

### Controllers

Four controllers registered via `pkg/controller/controller.go`:

| Controller | CRD | Purpose |
|---|---|---|
| `apischeme` | APIScheme | Manages admin API (`rh-api`) DNS/LB/security groups |
| `publishingstrategy` | PublishingStrategy | Controls API and ingress visibility (public/private) |
| `sshd` | SSHD | Manages SSH endpoint DNS/LB |
| `routerservice` | (watches Services) | Watches router service changes |

All CRDs are `v1alpha1` in API group `cloudingress.managed.openshift.io`, defined in `pkg/apis/cloudingress/v1alpha1/`.

### Key Files

- `cmd/manager/main.go` — Entry point, manager setup, leader election (`cloud-ingress-operator-lock`)
- `config/config.go` — Operator constants (names, namespaces, LB suffixes, secret names)
- `pkg/utils/infrastructure.go` — Cluster metadata helpers (base domain, cluster name, platform type)
- `pkg/localmetrics/` — Prometheus gauges for default ingress and APIScheme status

### Patterns Used

- **Finalizers**: Controllers use `CloudIngressFinalizer` and `ClusterIngressFinalizer` for cleanup before CR deletion
- **Standard reconciliation**: controller-runtime watch → fetch → compare → apply → update status → requeue
- **Multi-namespace watch**: Configured via `WATCH_NAMESPACE` env var (comma-separated)

## Testing

Tests use standard Go `testing` package with `sigs.k8s.io/controller-runtime/pkg/client/fake` for mock Kubernetes clients. Mock generation comment is in `pkg/cloudclient/cloudclient.go`:
```
mockgen -source=pkg/cloudclient/cloudclient.go -destination=pkg/cloudclient/mock_cloudclient/mock_cloudclient.go
```

## Code Generation

Generated files (`zz_generated.deepcopy.go`, `zz_generated.openapi.go`, CRD YAMLs in `deploy/crds/`) must be committed. Run `make generate` after modifying API types in `pkg/apis/`, then commit the generated output. `make validate` in CI will fail if generated files are stale.

## Boilerplate

This repo uses [openshift/boilerplate](https://github.com/openshift/boilerplate) for standardized build/test/lint infrastructure. Do not modify files under `boilerplate/` directly — they are managed upstream. Run `make boilerplate-update` to pull latest conventions.
