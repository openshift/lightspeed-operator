---
name: resolve-cve
description: Resolve a CVE vulnerability issue from Jira. Reads the CVE details, assesses impact on the Go-based operator, and either marks "not affected" with a Jira comment and transition, bumps the affected Go dependency, or implements a code fix. Use when the user says "cve", "resolve CVE", "vulnerability", or provides a CVE Jira issue key or URL.
---

# resolve-cve

The user provides a Jira key (e.g., `OLS-789`) or a Jira URL. If no specific issue is given, find CVEs to triage by searching the current sprint in the **OpenShift Lightspeed Service** (OLS) project:

```
project = OLS AND type = Vulnerability AND sprint in openSprints()
  AND (
    summary ~ "openshift-lightspeed/lightspeed-rhel9-operator"
    OR summary ~ "openshift-lightspeed/lightspeed-operator-bundle"
  )
  AND statusCategory = "To Do"
  ORDER BY priority DESC
```

Only process issues whose summary contains `openshift-lightspeed/lightspeed-rhel9-operator` or `openshift-lightspeed/lightspeed-operator-bundle` — these are the operator CVEs. Skip issues targeting other components (e.g., service, console plugin).

The summary format is: `CVE-YYYY-NNNNN openshift-lightspeed/{image}: {Package}: {Title} [ols-N]`

If using Jira MCP and the `cloudId` is unknown, call `getAccessibleAtlassianResources` to discover it, or ask the user.

## Step 1: Read the CVE Issue

Fetch the issue via `getJiraIssue` with `responseContentFormat: "markdown"`. The issue type is `Vulnerability`, not a regular story.

Parse the data from these locations:

- **CVE ID** — embedded in the `summary` field, e.g., `CVE-2026-33231 openshift-lightspeed/...: golang.org/x/net: ...`
- **Affected package** — mentioned in the description's `Flaw:` section (the description starts with boilerplate — "Security Tracking Issue", "Do not make this issue public" — skip to the flaw text after the `---` separator)
- **Vulnerable version range** — in the flaw prose
- **Fix reference** — upstream commit or PR link, if mentioned in the flaw text

Then look up severity externally:

- **CVSS score** — use `WebSearch` for the CVE ID on NVD (e.g., `CVE-2026-33231 NVD`) to get the severity rating

If the issue is missing a CVE ID or the affected package is unclear from the flaw text, ask the user to clarify.

## Step 2: Assess Impact

Determine whether this project is affected:

1. **Check if the package is a dependency** — search `go.mod` and `go.sum` for the package name. Go module paths are case-sensitive. If not present at all, the project is **not affected**.
2. **Check the installed version** — find the exact version in `go.mod` (direct) or `go.sum` (transitive). Compare against the vulnerable version range from the advisory.
3. **Check if the vulnerable code path is reachable** — if the CVE targets a specific function or package within the module, grep the codebase for imports of that specific sub-package. If the project never imports the affected package, it may be **not affected** even if the module is in the dependency tree. Also run `go mod why {sub-package}` (e.g., `go mod why golang.org/x/net/html`) to trace the full import chain — this reveals whether framework dependencies (like controller-runtime or gomega) pull in the vulnerable code transitively, even when the operator itself doesn't import it directly.
4. **Check transitive dependencies** — if the package isn't a direct dependency in `go.mod`, check whether it appears in `go.sum` or run `go mod graph | grep {package}` to trace which direct dependency pulls it in.

## Step 3: Present Assessment

Present the finding to the user clearly:

```
CVE Assessment: {CVE-ID}

Package: {Go module path}
Vulnerable versions: {range}
Installed version: {version from go.mod/go.sum}
Direct dependency: {yes/no — if no, pulled in by {parent}}

Verdict: {NOT AFFECTED / AFFECTED — bump needed / AFFECTED — code change needed}

Reasoning:
- {why this verdict — e.g., "module not in dependency tree",
  "installed version is outside vulnerable range",
  "vulnerable sub-package is not imported by this project",
  "project imports the affected package in internal/controller/..."}
```

**GATE — do not proceed without user acknowledgment.** The user may have context that changes the verdict (e.g., the package is used indirectly via generated code, or the feature is enabled in production but not in tests). Present the assessment and stop. Only continue after explicit "go".

## Step 4: Resolve

Based on the verdict and user acknowledgment:

### Path A: Not Affected

1. Add a comment to the Jira issue via `addCommentToJiraIssue` with `contentFormat: "markdown"`:

```
**Assessment: Not Affected**

{CVE-ID} targets {package} versions {range}.

{Reason — one of:}
- Module is not in the dependency tree.
- Installed version ({version}) is outside the
  vulnerable range.
- The vulnerable code path ({specific sub-package or
  function}) is not imported by this project.

No action required.
```

2. Transition the issue to **Done / Closed** with resolution **"Won't Do"**. Call `getTransitionsForJiraIssue` to find the transition ID for "Done" or "Closed", then `transitionJiraIssue` with that ID and `resolution: { name: "Won't Do" }` in the fields.

### Path B: Dependency Bump

1. **Bump the dependency:**
   - For a direct dependency: `go get {package}@{fixed-version}` (or `go get {package}@latest` if the latest version contains the fix).
   - For a transitive dependency: `go get {parent-package}@latest` to pull in the updated transitive dep. If the parent hasn't updated yet, try `go get {transitive-package}@{fixed-version}` directly.
   - Run `go mod tidy` to clean up.

2. **Verify the fix:**
   - Confirm the new version in `go.mod` / `go.sum` is outside the vulnerable range.
   - Run `make lint && make test` — both must pass.
   - If the latest release is still vulnerable, stop and tell the user — no fix is available upstream yet.

3. **Add a Jira comment:**

```
**Resolution: Dependency bumped**

{CVE-ID} targets {package} versions {range}.
Bumped {package} from {old version} to {new version}.

Lint/tests: passing.
```

4. Ask user about Jira transition (same as Path A step 2).

### Path C: Code Change (Rare)

1. Explain to the user what code change is needed and why. This is unusual — confirm the approach before implementing.
2. Make the targeted fix, write or update tests, and run `make lint && make test`.
3. Add a Jira comment summarizing the code change.
4. Ask user about Jira transition.

## Step 5: Report

```
CVE {CVE-ID} resolved for {story_id}.

Verdict: {Not Affected / Bumped {package} to {version} / Code fix applied}
Jira: {commented / commented + transitioned to {status}}

{If files changed:}
Files changed:
  - {list files}

Ready to commit.
{End if}
```

If the user wants a commit (Path B or C), use message:

```
fix: resolve {CVE-ID} — bump {package} to {version}
```

or for code changes:

```
fix: resolve {CVE-ID} — {brief description}
```

## Constraints

- **User acknowledgment required** — never act on the verdict without the user confirming the assessment. They may know things the codebase analysis cannot reveal.
- **Jira transitions** — Path A (Not Affected) transitions automatically to Done/Closed with resolution "Won't Do". For Paths B and C, ask the user which transition to use.
- **Minimal changes** — bump only the affected module, not all dependencies. Use targeted `go get`, not blanket `go get -u ./...`.
- **Verify after every change** — `make lint && make test` must pass before declaring done. Never use `go test` directly.
- **Do not downplay severity** — if the project is affected, say so clearly. Do not stretch "not affected" reasoning to avoid work.
