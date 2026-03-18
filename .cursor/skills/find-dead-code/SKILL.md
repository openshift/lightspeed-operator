---
name: find-dead-code
description: Find unused functions, types, constants, imports, and unreachable code paths. Use when the user asks to find dead code, unused code, cleanup candidates, or wants to reduce codebase size.
disable-model-invocation: true
---

# Find Dead Code

Detect unused Go code that can be safely removed.

## Rules

- Report findings, do not delete. Removal is a separate task.
- Focus on production code (`internal/`, `api/`, `cmd/`). Skip tests unless explicitly asked.
- Tools have false positives — classify each finding before recommending removal.
- Code used via reflection or dynamic dispatch (e.g. Kubernetes controller-runtime, interface implementations) is not dead.

## Step 1: Determine Scope

Ask the user:
- **Branch mode**: only files changed in the current branch vs main.
- **Full mode**: scan the entire codebase.

For branch mode:

```bash
git diff --name-only upstream/main -- 'internal/' 'api/' 'cmd/' | grep '\.go$' | grep -v '_test\.go$'
```

## Step 2: Run Go Standard Tooling

Check for unused code with the compiler:

```bash
go build ./... 2>&1 | grep "declared and not used"
go vet ./... 2>&1 | grep -E "(not used|never used)"
```

## Step 3: Run staticcheck

staticcheck detects unused code, including unexported functions, constants, and variables:

```bash
staticcheck -checks=U1000 ./...
```

U1000 reports unused code that is not exported and not referenced.

## Step 4: Run deadcode (Go 1.23+)

If Go 1.23+ is available:

```bash
go run golang.org/x/tools/cmd/deadcode@latest -filter <package-pattern>
```

This finds functions, types, and variables that are never called or referenced.

## Step 5: Check Unused Imports

```bash
goimports -l <target>
```

Any file listed has unused imports. Review with:

```bash
goimports -d <file>
```

## Step 6: Filter False Positives

Common false positives in this codebase:

| Pattern | Why it's not dead |
|---------|-------------------|
| `Reconcile(ctx, req)` implementations | Called by controller-runtime via interface |
| `SetupWithManager()` functions | Called by manager setup code |
| `init()` functions | Called automatically by Go runtime |
| Interface method implementations | Called through interface, not directly |
| `kubebuilder:` marker functions | Used by code generation |
| Constants/vars in `constants.go` | May be used in tests or future code |
| Error constants matching `Err*` pattern | May be used in error wrapping |

## Step 7: Classify Findings

For each finding, classify:

| Category | Criteria | Action |
|----------|----------|--------|
| **Remove** | Clearly unused, no interface/reflection use | Safe to delete |
| **Verify** | Possibly used dynamically or via interface | Search for references before removing |
| **False positive** | Interface impl, reflection, kubebuilder marker | Skip |

For "Verify" findings, search for references:

```bash
rg "<function_or_type_name>" internal/ api/ cmd/ test/
```

## Step 8: Report

For each finding:

1. File and line number
2. What is unused (function, type, constant, variable, import)
3. Tool that detected it (staticcheck, deadcode, goimports)
4. Classification (remove / verify / false positive)
5. Estimated lines saved

Summary: total findings, how many safe to remove, estimated cleanup size.
