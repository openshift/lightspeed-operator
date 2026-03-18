---
name: review-pr
description: Review PR with structured approach covering architecture, naming, patterns, and critical questions
disable-model-invocation: true
---

# Review PR

When asked to review a PR, follow this structured approach.

## 1. Fetch Latest Changes

**Always** fetch the latest PR state before reviewing. The cached PR data may be stale.

```bash
git fetch upstream pull/<PR_NUMBER>/head:pr-<PR_NUMBER>
git log pr-<PR_NUMBER> --oneline -10
git diff upstream/main...pr-<PR_NUMBER> --stat
```

For follow-up reviews, re-fetch to get new commits:

```bash
git fetch upstream pull/<PR_NUMBER>/head:pr-<PR_NUMBER> --force
```

Read diffs per area (controllers, reconcilers, assets, tests) rather than one massive diff.

## 2. Understand What It Implements

- Summarize the feature/fix in 2-3 sentences
- Identify the flow: entry point -> processing -> output
- Map which files serve which role (controller, reconciler, assets, CRD, test)

## 3. Evaluate How It's Implemented

Only raise issues if you have a concrete concern — not as a checklist to fill:

- **Architecture**: Is logic clearly in the wrong layer? (e.g. business logic leaking into controller/reconciler)
- **Error handling**: Are errors silently swallowed with `_ = err`, or is error wrapping missing where failure is plausible?
- **Duplication**: Is the same logic copy-pasted, not just similar-looking?
  - For deep analysis, use the [find-duplication](../find-duplication/SKILL.md) skill
- **Dead code / docs**: Is there obviously unused code or a doc update that's clearly missing?
  - For comprehensive analysis, use the [find-dead-code](../find-dead-code/SKILL.md) skill
- **Complexity**: Are functions overly complex with high cyclomatic/cognitive complexity?
  - For detailed analysis, use the [find-complexity](../find-complexity/SKILL.md) skill

Skip this section if nothing stands out.

## 4. Assess Naming

Only flag naming if it is genuinely misleading or inconsistent with established patterns in the codebase. Do not flag stylistic preferences or minor wording variations.

- Check against Go conventions: exported functions start with uppercase, package-level names don't stutter
- Verify interface names end in `-er` (Reader, Writer, Reconciler)

## 5. Check Go Patterns

**For production code**, load the [go-code-review](../go-code-review/SKILL.md) skill and apply its checklist:

- Error handling: All errors checked (no `_ = err`), errors wrapped with context (`%w`)
- Concurrency: Goroutines stoppable via context, channels closed by sender only
- Resource management: `defer Close()` immediately after opening resources
- Common mistakes: No string concatenation in loops, slices preallocated when size known
- Kubernetes operators: Owner references set, finalizers handled safely, context propagated

**For test code**, load the [go-testing-code-review](../go-testing-code-review/SKILL.md) skill and apply its checklist:

- Table-driven tests with clear case names
- Test helpers marked with `t.Helper()`
- Cleanup registered with `t.Cleanup()` or Ginkgo `AfterEach()`
- Error messages include both got and want values
- Parallel tests properly isolated (if using `t.Parallel()`)
- Ginkgo tests use proper `Describe`/`Context`/`It` structure
- Controller tests use `envtest` and handle eventual consistency

Only flag issues that cause real problems (correctness, maintainability, silent failures).

Skip this section if nothing meaningful to flag.

## 6. Ask Critical Questions

Only ask questions where the answer is genuinely unclear from the code and matters for correctness or design. Do not manufacture questions for completeness.

- What happens on invalid/missing/malformed input — if error paths are not visible in the diff?
- Are there security implications not addressed? (token logging, size limits, injection)
- Do tests cover behavior (specific assertions) or just confirm the code runs?
- Is the PR clearly bundling unrelated changes that should be in a separate PR?

Skip this section if the design is clear and the concerns above don't apply.

## 7. Verify Each Issue with a Subagent

Before writing the final output, launch one subagent per candidate issue to confirm it is real. Do this in parallel for all issues found in sections 3–6.

Each subagent task should:
- Receive the specific concern as its prompt (e.g. "Is error X silently swallowed? Check `foo.go` lines 40-55 and any callers.")
- Read the relevant file(s) and any related context (callers, tests, existing patterns in the codebase)
- Return a verdict: **confirmed** / **not an issue** / **unsure**, with a one-sentence rationale

After all subagents complete:
- Drop any issue whose verdict is **not an issue**
- Downgrade confidence to **unsure** for any issue whose verdict is **unsure**
- Only **confirmed** issues appear in the issues table at full confidence

This step exists to filter out false positives before they reach the output. Do not skip it when there are candidate issues.

## 8. Output Format

Structure the review as:

1. **Summary**: What the PR does (2-3 sentences)
2. **File-by-file analysis**: Role of each changed file (one line each; skip files with trivial changes)
3. **Issues table**: Only issues confirmed by subagent verification (section 7) — Priority (must-fix / should-fix / nice-to-have), issue, location, confidence. If there are no confirmed issues, say so explicitly.
4. **What's good**: Acknowledge well-done aspects — keep this genuine, not filler
5. **Critical questions**: Only if section 6 above produced anything; omit otherwise
6. **Closing reminder**: Always end the review with: *"You, as a human, need to evaluate these AI findings instead of just copying them as review comments. That would just shift the responsibility of validation to the PR creator."*
