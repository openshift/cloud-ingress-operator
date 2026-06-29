# Development Guide

Quick reference for developing the Cloud Ingress Operator.

## Prerequisites

- **Go**: 1.22.7 or later
- **operator-sdk**: v1.21.0
- **kubectl**: For cluster interaction
- **prek**: `uv tool install prek` (pre-commit hook manager)

## Initial Setup

```bash
# Clone repository
git clone https://github.com/openshift/cloud-ingress-operator.git
cd cloud-ingress-operator

# Install prek (pre-commit hook manager)
uv tool install prek      # recommended
# or: pipx install prek
# or: pip install --user prek

# Install git hooks
prek install
```

## Common Commands

### Build
```bash
make go-build                 # Build operator binary
make docker-build             # Build container image
```

### Test
```bash
make go-test                  # Run all unit tests
go test ./controllers/publishingstrategy/...  # Test specific package
ginkgo -r ./controllers/      # Run controller tests with Ginkgo
```

### Lint
```bash
make go-check                 # Full linting (golangci-lint)
prek run --all-files          # Run all prek hooks
prek run golangci-lint        # Lint only
```

### Code Generation
```bash
# After modifying API types (api/v1alpha1/*.go)
# or interfaces requiring mocks
boilerplate/_lib/container-make generate

# What this generates:
# - Deepcopy methods (zz_generated.deepcopy.go)
# - OpenAPI schemas
# - Mock interfaces for testing
```

### Run Locally
```bash
# Build and run the operator binary locally
make go-build
./build/_output/bin/cloud-ingress-operator

# Or use operator-sdk to run against cluster in ~/.kube/config
operator-sdk run local

# With verbose logging
operator-sdk run local --verbose
```

### Container-based Build
```bash
# Run make targets inside boilerplate container
# (ensures consistent environment with CI)
boilerplate/_lib/container-make
boilerplate/_lib/container-make go-test
boilerplate/_lib/container-make generate
```

## Fast Local Iteration

**Minimal validation loop:**
```bash
# After code changes
go build ./...                # Fast compile check (~5s)
go test ./pkg/mypackage       # Run affected tests
prek run                      # Lint staged files
```

**Full validation (pre-PR):**
```bash
prek run --all-files          # All hooks (~15-30s)
make go-test                  # Full test suite
```

## Targeted Testing

```bash
# Run specific test
ginkgo -focus="NetworkPolicy" ./controllers/publishingstrategy/

# Run tests for one package
go test -v ./controllers/publishingstrategy/

# Skip slow tests during development
ginkgo -skip="E2E" -r ./...
```

## Debugging

```bash
# Run operator with verbose logging
operator-sdk run local --verbose

# Print specific package logs
go test -v ./pkg/... 2>&1 | grep "MyFunction"

# Ginkgo verbose output
ginkgo -v ./...
```

## Dependency Management

```bash
# Add new dependency
go get github.com/some/package@v1.2.3

# Update dependency
go get -u github.com/some/package

# Tidy (removes unused, adds missing)
go mod tidy

# Verify checksums
go mod verify
```

**Note**: `go.sum` changes automatically trigger validation via prek hooks.

## Architecture Pointers

- **API Types**: `api/v1alpha1/` - CRD definitions
- **Controllers**: `controllers/{publishingstrategy,apischeme,routerservice}/` - Reconciliation logic
- **Business Logic**: `controllers/publishingstrategy/` - Resource management
- **Tests**: `*_test.go` alongside source, `*_suite_test.go` for Ginkgo
- **Mocks**: `pkg/cloudclient/mock_cloudclient/` - Generated mocks
- **E2E**: `test/e2e/` - End-to-end tests

## CI Parity

Local prek hooks mirror Tekton CI checks:
- **go-check** ↔ Tekton lint job
- **go-build** ↔ Compilation in CI
- **go-test** ↔ Unit test job
- **gitleaks** ↔ Security scanning

Run `prek run --all-files` before pushing to catch CI failures early.

## Boilerplate Integration

This repo uses Red Hat's standardized boilerplate:
- Centralized Makefiles: `boilerplate/openshift/golang-osd-operator/`
- Standard targets: `go-build`, `go-check`, `go-test`
- Container builds: `boilerplate/_lib/container-make`
- Update boilerplate: `make boilerplate-update`

## Troubleshooting

**Mock generation fails:**
```bash
# Use container-make for consistency with CI
boilerplate/_lib/container-make generate
```

**Pre-commit hook timeout:**
```bash
# macOS: Install GNU timeout
brew install coreutils

# Linux: timeout is built-in
```

**go.sum checksum mismatch:**
```bash
export GOPROXY="https://proxy.golang.org"
go mod tidy
```

**Tests fail locally but pass in CI:**
```bash
# Use container environment
boilerplate/_lib/container-make go-test
```

## Further Reading

- [Testing Guide](./TESTING.md)
- [Contributing Guide](./CONTRIBUTING.md)
- [E2E Testing](./test/e2e/README.md)
- [Operator SDK Docs](https://sdk.operatorframework.io/)
