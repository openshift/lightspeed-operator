---
name: validate-and-fix
description: Run the full validation pipeline (make test, make lint, optionally make test-e2e) and auto-fix trivial failures like formatting and import issues. Use when the user asks to validate, run tests, check the pipeline, or verify changes are clean.
disable-model-invocation: true
---

# Validate & Auto-Fix

Run the project validation pipeline, auto-fix trivial issues, and re-run until green or a real failure is found.

## Rules

- **CRITICAL**: Never use `go test` directly - ALWAYS use `make test` (handles envtest, CRDs, build flags)
- Never modify production logic to fix a test. Only fix test expectations, imports, formatting.
- Never skip or delete a failing test.
- Stop after 3 auto-fix cycles to avoid loops.
- Report real failures clearly; do not attempt speculative fixes.

## Step 1: Run Unit Tests

```bash
make test 2>&1 | tail -60
```

**Important**: The Makefile handles essential setup (envtest, CRDs, build flags) that `go test` doesn't.

If all pass, proceed to Step 3 (linting).
If failures occur, classify each failure (see Step 2).

## Step 2: Classify and Fix Failures

For each failure, determine its type:

**Auto-fixable** (fix immediately, then re-run Step 1):

| Type | Fix |
|------|-----|
| Test expects old constant value | Update assertion to match new value |
| Test uses renamed function | Update function name in test |
| Import error from refactor | Update import path |
| Missing cleanup in test | Add cleanup or use existing cleanup helpers |

**Real failures** (do not auto-fix):

- Logic errors in production code
- Assertion failures reflecting actual behavior regressions
- Reconciliation loop failures
- Context cancellation issues
- Failures in code you did not modify

For real failures: report the test name, file, error message, and stop.

## Step 3: Run Linting

```bash
make lint 2>&1 | tail -40
```

This runs: `golangci-lint`, `go fmt`, `go vet`, and custom checks.

If failures occur, apply the same classify-and-fix logic from Step 2.
Common fixes at this stage:

| Type | Fix |
|------|-----|
| `gofmt` formatting | `go fmt ./...` |
| Unused import | Remove the import |
| Unused variable | Remove the variable or use `_ = variable` if intentional |
| Missing error check | Add `if err != nil { return err }` |
| Ineffectual assignment | Remove or fix the assignment |

Re-run `make lint` after each fix. Proceed to Step 4 when green.

## Step 4: Run E2E Tests (Optional)

Only if the user explicitly asks or if changes affect reconciliation logic:

```bash
make test-e2e 2>&1 | tail -30
```

**Requirements**: Requires a running OpenShift/Kubernetes cluster with operator deployed.

E2E test failures are almost always real failures. Report and stop.

## Step 5: Check Bundle and Manifests

If API changes were made (`api/v1alpha1/`), regenerate manifests:

```bash
make generate
make manifests
git diff
```

If there are differences, commit them:

```bash
git add api/ config/
git commit -m "Regenerate manifests"
```

## Step 6: Report

Report exactly:

- `make test`: X passed / Y failed
  - List any failing tests with brief error summary
- `make lint`: pass/fail
  - List any remaining lint errors
- `make test-e2e`: (if run) X passed / Y failed
- Auto-fixes applied:
  - File: what was fixed (e.g., "internal/controller/utils/utils_test.go: updated assertion")
- Cycles used: N/3

If all green:

```
✅ All validation passed:
  - make test: all tests passing
  - make lint: no issues
  
Ready to commit or push.
```

Do not include unrelated diagnostics or suggestions.
