---
name: prow-ci
description: Access and analyze OpenShift Prow CI results for cloud-ingress-operator
trigger: prow, prow-ci, /prow-ci, ci results, check ci
---

# Prow CI Access for Cloud Ingress

This skill helps you access and analyze Prow CI results for the cloud-ingress-operator repository.

## Quick Start

```bash
# Invoke the skill
/prow-ci

# Or ask naturally:
"Check CI results for PR 255"
"What Prow jobs are failing?"
"Show me the CI status"
```

## Prow Resources

**Main Dashboard**: https://prow.ci.openshift.org/  
**CI Search**: https://github.com/openshift/ci-search  
**Job History**: https://prow.ci.openshift.org/?repo=openshift%2Fcloud-ingress-operator

## Common Use Cases

### 1. Check Recent CI Results

```bash
# View recent PR jobs
curl -s "https://prow.ci.openshift.org/?repo=openshift%2Fcloud-ingress-operator&type=presubmit" | grep -E "pull-ci-openshift-cloud-ingress-operator"

# Check latest job status for specific PR
# Replace PR_NUMBER with actual PR number
gh pr view PR_NUMBER --json statusCheckRollup --jq '.statusCheckRollup[] | select(.context | contains("prow"))'
```

### 2. Access Build Logs

Prow logs are stored at:
- **Pull request jobs**: `gs://test-platform-results/pr-logs/pull/openshift_cloud-ingress-operator/[PR_NUMBER]/[JOB_NAME]/[JOB_ID]`
- **Periodic jobs**: `gs://test-platform-results/logs/[JOB_NAME]/[JOB_ID]`

**Viewing logs via web**:
```text
https://prow.ci.openshift.org/view/gs/test-platform-results/pr-logs/pull/openshift_cloud-ingress-operator/[PR_NUMBER]/[JOB_NAME]/[JOB_ID]
```

### 3. Analyze Test Failures

```bash
# Get PR checks
gh pr view PR_NUMBER --json statusCheckRollup

# Find failed jobs
gh pr checks PR_NUMBER | grep -i "fail"

# Access specific job artifacts
# Navigate to Prow UI and click on:
# - Build Log (for compilation/test output)
# - JUnit (for structured test results)
# - Artifacts (for generated files, coverage, etc.)
```

### 4. Common Job Names

**Prow CI Jobs** (configured in openshift/release):
- `pull-ci-openshift-cloud-ingress-operator-master-e2e-binary-build-success` - E2E binary build verification
- `pull-ci-openshift-cloud-ingress-operator-master-coverage` - Code coverage analysis (with Codecov)
- `pull-ci-openshift-cloud-ingress-operator-master-lint` - Linting checks
- `pull-ci-openshift-cloud-ingress-operator-master-test` - Unit tests
- `pull-ci-openshift-cloud-ingress-operator-master-validate` - Validation checks

**Tekton Pipelines** (configured in `.tekton/`):
- `cloud-ingress-operator-pull-request` - Main PR pipeline (docker build with OCI-TA)
- `cloud-ingress-operator-e2e-pull-request` - E2E testing pipeline
- `cloud-ingress-operator-pko-pull-request` - PKO (Package Operator) pipeline
- Corresponding `-push` pipelines for merged commits

## Debugging CI Failures

### Step 1: Identify Failed Job
```bash
gh pr checks PR_NUMBER
```

### Step 2: Access Prow UI
Open the Prow link from PR checks or construct manually:
```text
https://prow.ci.openshift.org/?repo=openshift%2Fcloud-ingress-operator&type=presubmit
```

### Step 3: Review Logs
Click on failed job → "Build Log" tab

### Step 4: Check Artifacts
Look for:
- Test failure logs
- Coverage reports
- Generated artifacts

### Step 5: Reproduce Locally
Many Prow jobs can be reproduced with:
```bash
# For unit tests (matches: pull-ci-...-test)
make go-test

# For linting (matches: pull-ci-...-lint)
make go-check
# OR use pre-commit for comprehensive linting
pre-commit run --all-files

# For validation (matches: pull-ci-...-validate)
make validate

# For coverage (matches: pull-ci-...-coverage)
make coverage

# For E2E binary build (matches: pull-ci-...-e2e-binary-build-success)
make e2e-binary-build

# For container builds (Tekton pipelines)
make docker-build
```

## CI/Prow Integration in This Repo

This repo uses **both Prow and Tekton** for comprehensive CI:

**Prow CI** (openshift/release):
- Configuration: `ci-operator/config/openshift/cloud-ingress-operator/openshift-cloud-ingress-operator-master.yaml`
- Runs: lint, test, validate, coverage, e2e-binary-build
- Uses Codecov for coverage reporting (secret: `cloud-ingress-operator-codecov-token`)
- Skip rules: Changes to `.tekton/`, `.github/`, `.md` files, `OWNERS`, `LICENSE` don't trigger most jobs

**Tekton Pipelines** (`.tekton/`):
- Primary build pipeline using Pipelines as Code
- Three pipeline types: main, e2e, pko
- Builds container images to Quay (cloud-ingress-operator-tenant)
- Pull request images expire after 5 days
- Uses boilerplate framework from `openshift/boilerplate` (docker-build-oci-ta pipeline)

## Quick Reference Commands

```bash
# Check all PR checks status
gh pr checks <PR_NUMBER>

# View detailed status for a specific PR
gh pr view <PR_NUMBER> --json statusCheckRollup

# Filter only Prow jobs
gh pr checks <PR_NUMBER> | grep "pull-ci-openshift-cloud-ingress-operator"

# Check Tekton pipeline status
gh pr view <PR_NUMBER> --json statusCheckRollup --jq '.statusCheckRollup[] | select(.context | contains("Tekton"))'

# Open Prow dashboard in browser (cross-platform)
# Copy and paste this URL into your browser:
# https://prow.ci.openshift.org/?repo=openshift%2Fcloud-ingress-operator

# Or use platform-specific command:
# macOS: open "https://prow.ci.openshift.org/?repo=openshift%2Fcloud-ingress-operator"
# Linux: xdg-open "https://prow.ci.openshift.org/?repo=openshift%2Fcloud-ingress-operator"
# Windows: start "https://prow.ci.openshift.org/?repo=openshift%2Fcloud-ingress-operator"

# View specific PR on Prow (replace <PR_NUMBER>)
# https://prow.ci.openshift.org/?repo=openshift%2Fcloud-ingress-operator&type=presubmit&pull=<PR_NUMBER>
```

## Troubleshooting

### Can't find job results?
- Check both Prow AND Tekton - this repo uses both systems
- Prow jobs: `pull-ci-openshift-cloud-ingress-operator-master-*`
- Tekton jobs: Usually show as "Tekton" or pipeline names in PR checks
- Verify repo name format in Prow: `openshift_cloud-ingress-operator` (underscore, not dash)
- Ensure PR has been opened and CI has run

### Logs show permission denied?
- Prow logs are public for openshift org
- Use web UI (prow.ci.openshift.org) instead of gsutil
- Check if job ID is correct

### Job still running?
- Check Prow dashboard for in-progress jobs
- Look for "Pending" or "Running" status
- Wait for completion before accessing artifacts

### Tekton pipeline failures?
- Check the pipeline link in PR checks (usually links to Konflux/AppStudio UI)
- Tekton logs are in the AppStudio dashboard, not Prow
- Common issues:
  - Image build failures → Check Dockerfile syntax and build context
  - Pipeline timeout → Check for slow steps or network issues
  - Auth failures → Secret configuration in `cloud-ingress-operator-tenant` namespace
- Local validation:
  ```bash
  # Validate Tekton YAML syntax
  kubectl apply --dry-run=client -f .tekton/

  # Test container build locally
  podman build -f build/Dockerfile -t test:local .
  ```

## Advanced: CI Search

For historical job searches:
```bash
# Clone ci-search tool
git clone https://github.com/openshift/ci-search.git

# Use web interface at search.ci.openshift.org (if available)
# Search for patterns in build logs across all jobs
```

## References

- [Prow Dashboard](https://prow.ci.openshift.org/)
- [CI Search Tool](https://github.com/openshift/ci-search)
- [OpenShift CI Documentation](https://docs.ci.openshift.org/)

## CI Configuration Files

**Prow Configuration** (in openshift/release repo):
- Location: `ci-operator/config/openshift/cloud-ingress-operator/openshift-cloud-ingress-operator-master.yaml`
- Update process: Submit PR to openshift/release repository
- Auto-generated jobs in: `ci-operator/jobs/openshift/cloud-ingress-operator/`

**Tekton Pipelines** (in this repo):
- Location: `.tekton/` directory
- Files:
  - `cloud-ingress-operator-pull-request.yaml` - Main PR pipeline
  - `cloud-ingress-operator-push.yaml` - Post-merge pipeline
  - `cloud-ingress-operator-e2e-pull-request.yaml` - E2E testing
  - `cloud-ingress-operator-pko-pull-request.yaml` - PKO validation
- Triggered by: Pipelines as Code (via Tekton)
- Uses: Boilerplate docker-build-oci-ta pipeline from openshift/boilerplate

## Coverage Reporting

This repository uses Codecov for coverage tracking:
- Secret: `cloud-ingress-operator-codecov-token` (stored in Prow)
- Generate coverage locally: `make coverage`
- Coverage runs on PRs and post-merge (`publish-coverage`)
- Dashboard: Check Codecov for cloud-ingress-operator

## Integration with Other Skills

- Use with **test-agent** to compare local test results with CI
- Use with **ci-agent** to validate CI configuration
- Use with **lint-agent** when investigating lint failures in CI
- Use with **security-agent** when investigating pre-commit hook failures
