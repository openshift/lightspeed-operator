---
name: find-duplication
description: Find code duplication in the codebase. Supports two modes - scoped to current branch changes or a full codebase sweep. Use when the user asks to find duplicated code, copy-paste, repeated patterns, or wants to deduplicate before a PR.
disable-model-invocation: true
---

# Find Code Duplication

Detect duplicated or near-duplicate Go code and suggest consolidation candidates.

## Rules

- Report findings, do not refactor. Refactoring is a separate task.
- Focus on production code (`internal/`, `api/`, `cmd/`). Skip test duplication unless explicitly asked.
- Group findings by severity: exact duplicates first, then near-duplicates.
- For each finding, state whether extraction is worth it or acceptable duplication.

## Step 1: Determine Scope

Ask the user:
- **Branch mode**: only files changed in the current branch vs main.
- **Full mode**: scan the entire codebase.

For branch mode:

```bash
git diff --name-only upstream/main -- 'internal/' 'api/' 'cmd/' | grep '\.go$' | grep -v '_test\.go$'
```

For full mode, the target is `internal/ api/ cmd/`.

## Step 2: Run dupl

Install and run dupl to find duplicate code blocks:

```bash
go install github.com/mibk/dupl@latest
dupl -threshold 15 <target>
```

`-threshold 15` means at least 15 tokens of duplication. Lower values = more noise.

Review output and filter false positives:
- Import blocks (common imports are not duplication)
- Error constant declarations (repeated pattern is intentional)
- Single-line patterns (logging, error wrapping)
- Kubebuilder boilerplate (RBAC markers, webhook setup)

## Step 3: Semantic Duplication Search

dupl only catches textual similarity. Also look for:

1. **Similar function signatures** — functions with near-identical parameter lists doing similar work across different packages.
2. **Repeated error handling** — same `if err != nil { return fmt.Errorf(...) }` pattern with slight variations.
3. **Copy-pasted reconciler logic** — similar reconciliation patterns across different controllers.
4. **Duplicated struct definitions** — similar structs in different packages (candidate for shared types).

Search for patterns:

```bash
# Find similar error wrapping
rg "fmt\.Errorf.*%w.*err\)" internal/ -A 1 -B 1

# Find similar reconciler patterns
rg "func.*Reconcile.*\(r reconciler\.Reconciler" internal/ -l

# Find similar asset generation
rg "func Generate.*\(r reconciler\.Reconciler" internal/ -l
```

## Step 4: Check for Repeated Utilities

Look for utility functions that appear in multiple packages:

```bash
# Find GetSecretContent-like patterns
rg "func.*GetSecret" internal/ -l

# Find generation helpers
rg "func.*Generate.*Labels" internal/ -l

# Find comparison helpers
rg "func.*Equal\(" internal/ -l
```

These should be in `internal/controller/utils/` or similar shared locations.

## Step 5: Classify Findings

For each duplicate found, classify:

| Category | Action |
|----------|--------|
| **Extract** — identical logic in 3+ places | Recommend a shared helper in utils |
| **Parameterize** — same structure, different values | Recommend a common function with parameters |
| **Acceptable** — similar but serving different domains (e.g. appserver vs postgres) | Note it, no action needed |
| **Boilerplate** — kubebuilder/controller-runtime patterns | Skip, this is framework convention |
| **Test-only** — repeated test setup/fixtures | Recommend shared test fixture (only if user asked) |

## Step 6: Report

For each finding:

1. Files and line ranges involved
2. What is duplicated (brief description)
3. Token/line count
4. Classification (extract / parameterize / acceptable / boilerplate)
5. Suggested location for shared code (e.g., `internal/controller/utils/`)

Summary: total findings, how many actionable, estimated lines saved.
