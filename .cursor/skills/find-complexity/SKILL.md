---
name: find-complexity
description: Find functions with high cyclomatic complexity, excessive length, or too many parameters. Use when the user asks to find complex code, complexity hotspots, refactoring candidates, or wants to improve code maintainability.
disable-model-invocation: true
---

# Find Complexity Hotspots

Identify Go functions that are hard to review, test, and maintain.

## Rules

- Report findings, do not refactor. Refactoring is a separate task.
- Focus on production code (`internal/`, `api/`, `cmd/`). Skip tests unless explicitly asked.
- Rank by severity: highest complexity first.

## Step 1: Determine Scope

Ask the user:
- **Branch mode**: only files changed in the current branch vs main.
- **Full mode**: scan the entire codebase.

For branch mode:

```bash
git diff --name-only upstream/main -- 'internal/' 'api/' 'cmd/' | grep '\.go$' | grep -v '_test\.go$'
```

## Step 2: Install Prerequisites

Install gocyclo and gocognit if not available:

```bash
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
go install github.com/uudashr/gocognit/cmd/gocognit@latest
```

## Step 3: Cyclomatic Complexity

```bash
gocyclo -over 10 <target>
```

This shows functions with cyclomatic complexity over 10.

Thresholds: 1-10 (simple), 11-20 (moderate), 21-50 (complex), 51+ (untestable).

## Step 4: Cognitive Complexity

Cognitive complexity weights nesting depth — a 5-deep `if` scores much higher than 5 sequential `if`s.

```bash
gocognit -over 15 <target>
```

This shows functions with cognitive complexity over 15.

## Step 5: Function Length

Find long functions (50+ lines of code, excluding comments and blank lines):

```bash
for file in $(find <target> -name '*.go' -not -name '*_test.go'); do
    awk '/^func / {start=NR; func=$0} 
         /^}/ && start {
           len=NR-start; 
           if(len>50) print FILENAME":"start": "func" ("len" lines)"
         }' "$file"
done
```

## Step 6: Parameter Count

Find functions with too many parameters (6+):

```bash
rg "^func.*\([^)]{60,}\)" <target> -A 0
```

Functions with 6+ parameters are candidates for parameter objects or config structs.

## Step 7: File Size

Find large files (500+ lines):

```bash
wc -l $(find <target> -name '*.go' -not -name '*_test.go') | sort -rn | head -20
```

Files over 500 lines are candidates for splitting into focused packages.

## Step 8: Classify Findings

For each function found, classify:

| Category | Criteria | Action |
|----------|----------|--------|
| **Split** | High complexity + long body | Break into smaller functions |
| **Simplify** | High complexity + short body | Reduce branching (early returns, switch statements) |
| **Parameterize** | Too many arguments (6+) | Group into config struct |
| **Monitor** | Complexity 11-15, not growing | Note it, revisit if it gets worse |
| **Split file** | File over 500 lines | Break into focused packages |

## Step 9: Report

For each finding:

1. File, function name, line number
2. Cyclomatic/cognitive complexity score
3. Lines of code / parameter count
4. Classification (split / simplify / parameterize / monitor)
5. Brief suggestion

Summary: total hotspots, top 5 worst offenders, estimated refactoring effort.
