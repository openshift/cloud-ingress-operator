# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

The cloud-ingress-operator is a Kubernetes operator designed for OpenShift Dedicated 4.x clusters to toggle cluster components between "private" and "public" modes. It manages:

1. **API Server Access**: Default API endpoint (`api.<cluster-domain>`) and admin API endpoint (`rh-api.<cluster-domain>`)
2. **Application Ingress**: Default ingress (`*.apps.<cluster-domain>`) and optional secondary ingress (`*.apps2.<cluster-domain>`)

The operator uses custom resources `APIScheme` and `PublishingStrategy` to control these behaviors and supports both AWS and GCP cloud providers.

## Commands

### Development Commands
```bash
# Build the operator binary
make go-build

# Run tests
make go-test

# Run linting and static analysis
make go-check

# Generate code (CRDs, deepcopy, OpenAPI)
make generate

# Validate all code and configurations
make validate

# Run comprehensive linting
make lint

# Docker build
make docker-build

# Build and push container
make docker-push
```

### Testing Commands
```bash
# Run unit tests with coverage
make go-test

# Run container tests
make container-test

# Generate test coverage reports
make coverage

# Validate YAML configurations
make yaml-validate
```

## Architecture

### Core Components

**Controllers** (`controllers/`):
- `apischeme/`: Manages admin API endpoint creation and configuration
- `publishingstrategy/`: Handles privacy toggling for API and ingress resources
- `routerservice/`: Manages router service configurations

**Cloud Clients** (`pkg/cloudclient/`):
- Abstract cloud provider interface with AWS and GCP implementations
- Handles load balancer and security group management
- Provider-specific networking configurations in `aws/` and `gcp/` subdirectories

**Custom Resources** (`api/v1alpha1/`):
- `APIScheme`: Configures admin API endpoint (`managementAPIServerIngress`)
- `PublishingStrategy`: Controls privacy settings for `defaultAPIServerIngress` and `applicationIngress`

### Key Dependencies

- **Operator SDK**: Built using controller-runtime framework
- **OpenShift APIs**: Integrates with OpenShift infrastructure and ingress controllers
- **Cloud SDKs**: AWS SDK and Google Cloud APIs for infrastructure management
- **Boilerplate**: Uses OpenShift boilerplate for standardized build/test/deploy patterns

### Important Patterns

- **Multi-cloud Support**: Conditional compilation and runtime detection for AWS vs GCP
- **FIPS Compliance**: Configurable FIPS mode for cryptographic operations
- **Legacy Support**: Feature flags for managing deprecated `applicationIngress` functionality
- **Version-aware Logic**: Cluster version detection for compatibility (especially OCP 4.13+ changes)

## Development Notes

- The operator runs with elevated permissions across multiple namespaces: `openshift-cloud-ingress-operator`, `openshift-ingress`, `openshift-ingress-operator`, `openshift-kube-apiserver`, `openshift-machine-api`
- Testing requires careful setup due to dependencies on cloud infrastructure and OpenShift-specific resources
- Manual testing instructions are provided in README.md for fleet deployments
- The project uses generated includes from boilerplate conventions for consistent build processes