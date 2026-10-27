package hack

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The tests below exercise hack/codecov.sh, which uploads Go coverage
// profiles to codecov.io from prow CI. The script must soft-fail (never
// block CI) but clearly log whether the upload succeeded, failed, or was
// skipped. A stubbed codecovcli binary on PATH keeps the tests offline.

const (
	prSHA   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	baseSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	token   = "test-codecov-token"
)

const cliStub = `#!/usr/bin/env bash
count="$(cat "$STUB_CALL_COUNT_FILE" 2> /dev/null || echo 0)"
count=$((count + 1))
printf '%s\n' "$count" > "$STUB_CALL_COUNT_FILE"
printf '%s\n' "$@" > "$STUB_ARGS_FILE"
if [ "${STUB_FAIL_FIRST_CALL:-0}" = "1" ] && [ "$count" -eq 1 ]; then
    exit 1
fi
exit "${STUB_EXIT_CODE:-0}"
`

// codecovScriptResult captures the outcome of one hack/codecov.sh run,
// including the directory holding the codecovcli stub's recorded files.
type codecovScriptResult struct {
	stdout   string
	exitCode int
	// dir is the temporary directory holding stub_args.txt and stub_calls.txt.
	dir string
}

// stubArguments returns the arguments the stubbed codecovcli was invoked with.
func (r codecovScriptResult) stubArguments(t *testing.T) []string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(r.dir, "stub_args.txt"))
	if err != nil {
		t.Fatalf("codecovcli stub was never invoked: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

// stubCallCount returns how many times the stubbed codecovcli was invoked.
func (r codecovScriptResult) stubCallCount(t *testing.T) int {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(r.dir, "stub_calls.txt"))
	if err != nil {
		t.Fatalf("codecovcli stub was never invoked: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		t.Fatalf("invalid stub call count %q: %v", string(content), err)
	}
	return count
}

// stubWasInvoked reports whether the stubbed codecovcli ran at all.
func (r codecovScriptResult) stubWasInvoked() bool {
	if _, err := os.Stat(filepath.Join(r.dir, "stub_calls.txt")); err != nil {
		return false
	}
	return true
}

// runCodecovScript runs hack/codecov.sh with a stubbed codecovcli on PATH
// and the given environment overrides applied on top of a prow-like
// presubmit environment. The coverage report is a minimal placeholder file.
func runCodecovScript(t *testing.T, overrides map[string]string) codecovScriptResult {
	t.Helper()

	dir := t.TempDir()
	stubDir := filepath.Join(dir, "bin")
	artifactsDir := filepath.Join(dir, "artifacts")
	for _, d := range []string{stubDir, artifactsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", d, err)
		}
	}
	stub := filepath.Join(stubDir, "codecovcli")
	if err := os.WriteFile(stub, []byte(cliStub), 0o755); err != nil {
		t.Fatalf("failed to write codecovcli stub: %v", err)
	}
	reportFile := filepath.Join(dir, "cover.out")
	if err := os.WriteFile(reportFile, []byte("mode: atomic\n"), 0o644); err != nil {
		t.Fatalf("failed to write coverage report: %v", err)
	}

	env := map[string]string{
		"PATH":                        stubDir + ":" + os.Getenv("PATH"),
		"JOB_TYPE":                    "presubmit",
		"CODECOV_TOKEN":               token,
		"PULL_NUMBER":                 "1234",
		"PULL_PULL_SHA":               prSHA,
		"PULL_BASE_SHA":               baseSHA,
		"PULL_BASE_REF":               "main",
		"REPO_OWNER":                  "openshift",
		"REPO_NAME":                   "lightspeed-operator",
		"ARTIFACT_DIR":                artifactsDir,
		"STUB_ARGS_FILE":              filepath.Join(dir, "stub_args.txt"),
		"STUB_CALL_COUNT_FILE":        filepath.Join(dir, "stub_calls.txt"),
		"STUB_FAIL_FIRST_CALL":        "0",
		"STUB_EXIT_CODE":              "0",
		"CODECOV_RETRY_SLEEP_SECONDS": "0",
	}
	for key, value := range overrides {
		env[key] = value
	}

	cmd := exec.Command("./codecov.sh", reportFile, "unit") // #nosec G204
	cmd.Env = mergedEnv(env)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	result := codecovScriptResult{dir: dir}
	if err := cmd.Run(); err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run codecov.sh: %v", err)
		}
		result.exitCode = exitError.ExitCode()
	}
	result.stdout = stdout.String()
	return result
}

// mergedEnv returns os.Environ with the given overrides applied.
func mergedEnv(overrides map[string]string) []string {
	applied := make(map[string]bool)
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if value, ok := overrides[key]; ok {
			env = append(env, key+"="+value)
			applied[key] = true
			continue
		}
		env = append(env, entry)
	}
	for key, value := range overrides {
		if !applied[key] {
			env = append(env, key+"="+value)
		}
	}
	return env
}

// optionValue returns the value following the given CLI option.
func optionValue(t *testing.T, args []string, option string) string {
	t.Helper()
	for i, arg := range args {
		if arg == option {
			if i+1 >= len(args) {
				t.Fatalf("option %s has no value", option)
			}
			return args[i+1]
		}
	}
	t.Fatalf("option %s not found in %v", option, args)
	return ""
}

// TestPresubmitUploadPassesReferenceFlagsAsSeparateArguments guards against
// passing PR number, commit SHA and branch as a single quoted argument, which
// previously broke codecov association in lightspeed-service (OLS-4180).
func TestPresubmitUploadPassesReferenceFlagsAsSeparateArguments(t *testing.T) {
	result := runCodecovScript(t, nil)

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	args := result.stubArguments(t)
	if got := optionValue(t, args, "--pr"); got != "1234" {
		t.Errorf("expected PR number 1234, got %q", got)
	}
	if got := optionValue(t, args, "--sha"); got != prSHA {
		t.Errorf("expected PR head SHA, got %q", got)
	}
	if got := optionValue(t, args, "--branch"); got != "main" {
		t.Errorf("expected branch main, got %q", got)
	}
}

// TestPresubmitUploadPassesCommonArguments verifies repository, token,
// report file and flag metadata on presubmit uploads.
func TestPresubmitUploadPassesCommonArguments(t *testing.T) {
	result := runCodecovScript(t, nil)

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	args := result.stubArguments(t)
	if !contains(args, "upload-process") {
		t.Errorf("expected upload-process subcommand, got %v", args)
	}
	if got := optionValue(t, args, "--git-service"); got != "github" {
		t.Errorf("expected git service github, got %q", got)
	}
	if got := optionValue(t, args, "--slug"); got != "openshift/lightspeed-operator" {
		t.Errorf("expected slug openshift/lightspeed-operator, got %q", got)
	}
	if got := optionValue(t, args, "--token"); got != token {
		t.Errorf("expected upload token, got %q", got)
	}
	if got := optionValue(t, args, "--flag"); got != "unit" {
		t.Errorf("expected flag unit, got %q", got)
	}
	if !contains(args, "--fail-on-error") {
		t.Errorf("expected --fail-on-error, got %v", args)
	}
	if !strings.Contains(result.stdout, "upload succeeded") {
		t.Errorf("expected success log, got: %s", result.stdout)
	}
}

// TestPostsubmitUploadUsesBaseRefAndBaseSHA verifies postsubmit uploads use
// the base SHA and carry no pull request number.
func TestPostsubmitUploadUsesBaseRefAndBaseSHA(t *testing.T) {
	result := runCodecovScript(t, map[string]string{"JOB_TYPE": "postsubmit"})

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	args := result.stubArguments(t)
	if got := optionValue(t, args, "--sha"); got != baseSHA {
		t.Errorf("expected base SHA, got %q", got)
	}
	if got := optionValue(t, args, "--branch"); got != "main" {
		t.Errorf("expected branch main, got %q", got)
	}
	for _, arg := range args {
		if arg == "--pr" {
			t.Errorf("postsubmit upload must not carry --pr, got %v", args)
		}
	}
}

// TestUploadFailureDoesNotBlockCI verifies a failing upload exits 0 and logs
// the failure loudly.
func TestUploadFailureDoesNotBlockCI(t *testing.T) {
	result := runCodecovScript(t, map[string]string{
		"STUB_EXIT_CODE":       "1",
		"CODECOV_MAX_ATTEMPTS": "1",
	})

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	if !strings.Contains(result.stdout, "CODECOV UPLOAD FAILED") {
		t.Errorf("expected failure banner, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "not blocked") {
		t.Errorf("expected soft-fail note, got: %s", result.stdout)
	}
}

// TestTransientFailureIsRetriedAndSucceeds verifies the script default retry
// count (2 attempts) recovers from a one-time codecov failure.
func TestTransientFailureIsRetriedAndSucceeds(t *testing.T) {
	result := runCodecovScript(t, map[string]string{"STUB_FAIL_FIRST_CALL": "1"})

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	if calls := result.stubCallCount(t); calls != 2 {
		t.Errorf("expected 2 codecovcli calls, got %d", calls)
	}
	if !strings.Contains(result.stdout, "upload succeeded") {
		t.Errorf("expected success log, got: %s", result.stdout)
	}
}

// TestLocalRunSkipsUpload verifies local runs do not upload and say so.
func TestLocalRunSkipsUpload(t *testing.T) {
	result := runCodecovScript(t, map[string]string{"JOB_TYPE": "local"})

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	if result.stubWasInvoked() {
		t.Error("local run must not invoke codecovcli")
	}
	if !strings.Contains(result.stdout, "skipped") {
		t.Errorf("expected skip log, got: %s", result.stdout)
	}
}

// TestMissingTokenSkipsUpload verifies a missing CODECOV_TOKEN is reported
// without blocking CI.
func TestMissingTokenSkipsUpload(t *testing.T) {
	result := runCodecovScript(t, map[string]string{"CODECOV_TOKEN": ""})

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	if result.stubWasInvoked() {
		t.Error("missing token must not invoke codecovcli")
	}
	if !strings.Contains(result.stdout, "CODECOV UPLOAD FAILED") {
		t.Errorf("expected failure banner, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "CODECOV_TOKEN") {
		t.Errorf("expected token to be named as the reason, got: %s", result.stdout)
	}
}

// contains reports whether the slice contains the given string.
func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
