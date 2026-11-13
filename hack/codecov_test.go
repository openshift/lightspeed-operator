package hack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
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
if [ -n "${STUB_CHILD_PID_FILE:-}" ]; then
    sleep 30 &
    child_pid=$!
    printf '%s\n' "$child_pid" > "$STUB_CHILD_PID_FILE"
    wait "$child_pid"
fi
if [ -n "${STUB_SLEEP_SECONDS:-}" ]; then
    sleep "$STUB_SLEEP_SECONDS"
fi
if [ "${STUB_FAIL_FIRST_CALL:-0}" = "1" ] && [ "$count" -eq 1 ]; then
    exit 1
fi
exit "${STUB_EXIT_CODE:-0}"
`

// The tests deliberately do not stub out `timeout`: a fake forwarding stub
// hides macOS, where GNU timeout does not exist, so the timeout path would go
// untested on a developer machine.

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
	return runCodecovScriptWithStubs(t, overrides, scriptStubs{})
}

// scriptStubs customizes the PATH the script under test sees.
type scriptStubs struct {
	// extra executables written into the stub directory, keyed by file name.
	extra map[string]string
	// withoutCLI leaves codecovcli off PATH so resolve_cli performs its own
	// lookup instead of returning on the first line.
	withoutCLI bool
}

// runCodecovScriptWithStubs is runCodecovScript plus the ability to fake the
// host (a `uname` reporting Darwin) or an absent codecovcli.
func runCodecovScriptWithStubs(t *testing.T, overrides map[string]string, stubs scriptStubs) codecovScriptResult {
	t.Helper()

	dir := t.TempDir()
	stubDir := filepath.Join(dir, "bin")
	artifactsDir := filepath.Join(dir, "artifacts")
	for _, d := range []string{stubDir, artifactsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", d, err)
		}
	}
	if !stubs.withoutCLI {
		if err := os.WriteFile(filepath.Join(stubDir, "codecovcli"), []byte(cliStub), 0o755); err != nil {
			t.Fatalf("failed to write codecovcli stub: %v", err)
		}
	}
	for name, content := range stubs.extra {
		if err := os.WriteFile(filepath.Join(stubDir, name), []byte(content), 0o755); err != nil {
			t.Fatalf("failed to write %s stub: %v", name, err)
		}
	}
	reportFile := filepath.Join(dir, "cover.out")
	if err := os.WriteFile(reportFile, []byte("mode: atomic\n"), 0o644); err != nil {
		t.Fatalf("failed to write coverage report: %v", err)
	}

	path := stubDir + string(os.PathListSeparator) + os.Getenv("PATH")
	if stubs.withoutCLI {
		path = stubDir + string(os.PathListSeparator) + pathWithoutCLI(t)
	}

	env := map[string]string{
		"PATH":                        path,
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

// TestUploadTimeoutStopsCLIAndSoftFails verifies a hung upload is stopped at
// the configured timeout instead of stalling CI until the job is killed, and
// still soft-fails.
func TestUploadTimeoutStopsCLIAndSoftFails(t *testing.T) {
	started := time.Now()
	result := runCodecovScript(t, map[string]string{
		"CODECOV_MAX_ATTEMPTS":    "1",
		"CODECOV_TIMEOUT_SECONDS": "1",
		"STUB_SLEEP_SECONDS":      "30",
	})
	elapsed := time.Since(started)

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	if calls := result.stubCallCount(t); calls != 1 {
		t.Errorf("expected 1 codecovcli call, got %d", calls)
	}
	if elapsed > 8*time.Second {
		t.Errorf("expected the upload to stop after ~1s, took %s", elapsed)
	}
	if !strings.Contains(result.stdout, "CODECOV UPLOAD FAILED") {
		t.Errorf("expected failure banner, got: %s", result.stdout)
	}
}

// TestUploadTimeoutKillsCLIDescendants verifies a timed-out upload does not
// leave the processes spawned by the CLI running in the background.
func TestUploadTimeoutKillsCLIDescendants(t *testing.T) {
	childPIDFile := filepath.Join(t.TempDir(), "child.pid")
	result := runCodecovScript(t, map[string]string{
		"CODECOV_MAX_ATTEMPTS":    "1",
		"CODECOV_TIMEOUT_SECONDS": "1",
		"STUB_CHILD_PID_FILE":     childPIDFile,
	})

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	content, err := os.ReadFile(childPIDFile)
	if err != nil {
		t.Fatalf("codecovcli stub never recorded a child pid: %v", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		t.Fatalf("invalid child pid %q: %v", string(content), err)
	}

	waitForProcessExit(t, childPID)
}

// TestCancelledPerlHelperKillsCommandGroup verifies the Perl fallback used on
// hosts without GNU timeout stops the command and its descendants when CI
// cancels the step.
func TestCancelledPerlHelperKillsCommandGroup(t *testing.T) {
	perl, err := exec.LookPath("perl")
	if err != nil {
		t.Skip("perl is not installed, so the timeout fallback cannot be exercised")
	}
	pidFile := filepath.Join(t.TempDir(), "pids.txt")
	cmd := exec.Command( // #nosec G204
		perl,
		"run_with_timeout.pl",
		"30",
		"bash",
		"-c",
		`sleep 30 & echo $$ $! > "$1"; wait`,
		"helper-test",
		pidFile,
	)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start run_with_timeout.pl: %v", err)
	}

	pids := waitForPIDFile(t, pidFile, 2)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to signal run_with_timeout.pl: %v", err)
	}
	err = cmd.Wait()
	exitError, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected a non-zero exit after cancellation, got %v", err)
	}
	if code := exitError.ExitCode(); code != 128+int(syscall.SIGTERM) {
		t.Errorf("expected exit code %d, got %d: %s", 128+int(syscall.SIGTERM), code, stdout.String())
	}
	for _, pid := range pids {
		waitForProcessExit(t, pid)
	}
}

// waitForPIDFile polls pidFile until it holds want pids or the deadline passes.
func waitForPIDFile(t *testing.T, pidFile string, want int) []int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(pidFile)
		if err == nil {
			fields := strings.Fields(string(content))
			if len(fields) == want {
				pids := make([]int, 0, want)
				for _, field := range fields {
					pid, err := strconv.Atoi(field)
					if err != nil {
						t.Fatalf("invalid pid %q: %v", field, err)
					}
					pids = append(pids, pid)
				}
				return pids
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("processes did not start within the deadline")
	return nil
}

// waitForProcessExit polls until the pid is gone or has become a zombie. A
// zombie still holds the pid but no longer runs code, so it counts as exited.
func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	status := ""
	for time.Now().Before(deadline) {
		out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output() // #nosec G204
		if err != nil {
			return
		}
		status = strings.TrimSpace(string(out))
		if status == "" || strings.HasPrefix(status, "Z") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("process %d is still running (state %q) after the timeout", pid, status)
}

// unameStubFor reports the given OS and architecture so a test can pretend to
// run on another host.
func unameStubFor(goos, arch string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
case "${1:-}" in
    -s) echo %s ;;
    -m) echo %s ;;
    *) echo %s ;;
esac
`, goos, arch, goos)
}

// curlStub records how the script invoked curl and writes an executable
// stand-in for the downloaded CLI.
const curlStub = `#!/usr/bin/env bash
printf '%s\n' "$@" > "$CURL_ARGS_FILE"
dest=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o) dest="$2"; shift 2 ;;
        *) shift ;;
    esac
done
cat > "$dest" <<'FAKE_CLI'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$FAKE_CLI_ARGS_FILE"
exit 0
FAKE_CLI
chmod +x "$dest"
`

// scriptCLIVersion returns the codecov CLI version pinned by codecov.sh so the
// tests do not hardcode it.
func scriptCLIVersion(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("codecov.sh")
	if err != nil {
		t.Fatalf("failed to read codecov.sh: %v", err)
	}
	match := regexp.MustCompile(`(?m)^CODECOV_CLI_VERSION="([^"]+)"`).FindSubmatch(content)
	if match == nil {
		t.Fatal("codecov.sh does not pin CODECOV_CLI_VERSION")
	}
	return string(match[1])
}

// TestCLIAssetMatchesHostPlatform verifies the downloaded CLI asset is chosen
// from the OS and the architecture, so a Mac no longer fetches a Linux binary
// it cannot execute.
func TestCLIAssetMatchesHostPlatform(t *testing.T) {
	cases := []struct {
		goos      string
		arch      string
		wantAsset string
	}{
		{"Linux", "x86_64", "codecovcli_linux"},
		{"Linux", "aarch64", "codecovcli_linux_arm64"},
		{"Linux", "arm64", "codecovcli_linux_arm64"},
		{"Darwin", "arm64", "codecovcli_macos"},
		{"Darwin", "x86_64", "codecovcli_macos"},
	}

	version := scriptCLIVersion(t)
	for _, tc := range cases {
		t.Run(tc.goos+"/"+tc.arch, func(t *testing.T) {
			curlArgsFile := filepath.Join(t.TempDir(), "curl_args.txt")
			cliArgsFile := filepath.Join(t.TempDir(), "cli_args.txt")
			result := runCodecovScriptWithStubs(t,
				map[string]string{
					"TMPDIR":             t.TempDir(),
					"CURL_ARGS_FILE":     curlArgsFile,
					"FAKE_CLI_ARGS_FILE": cliArgsFile,
				},
				scriptStubs{withoutCLI: true, extra: map[string]string{
					"uname": unameStubFor(tc.goos, tc.arch),
					"curl":  curlStub,
				}},
			)

			if result.exitCode != 0 {
				t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
			}
			curlArgs, err := os.ReadFile(curlArgsFile)
			if err != nil {
				t.Fatalf("the CLI was not downloaded: %v", err)
			}
			wantURL := "/v" + version + "/" + tc.wantAsset
			if !strings.Contains(string(curlArgs), wantURL) {
				t.Errorf("expected to download %q on %s/%s, curl got %q", wantURL, tc.goos, tc.arch, string(curlArgs))
			}
			if !strings.Contains(result.stdout, "downloading "+tc.wantAsset) {
				t.Errorf("expected the download to be logged, got: %s", result.stdout)
			}
			cliArgs, err := os.ReadFile(cliArgsFile)
			if err != nil {
				t.Fatalf("the downloaded CLI was never invoked: %v", err)
			}
			if !strings.Contains(string(cliArgs), "upload-process") {
				t.Errorf("expected the downloaded CLI to receive the upload arguments, got %q", string(cliArgs))
			}
		})
	}
}

// TestUnsupportedPlatformSoftFails checks that a host with no published CLI
// asset reports why instead of downloading something that cannot run.
func TestUnsupportedPlatformSoftFails(t *testing.T) {
	curlArgsFile := filepath.Join(t.TempDir(), "curl_args.txt")
	result := runCodecovScriptWithStubs(t,
		map[string]string{"TMPDIR": t.TempDir(), "CURL_ARGS_FILE": curlArgsFile},
		scriptStubs{withoutCLI: true, extra: map[string]string{
			"uname": unameStubFor("FreeBSD", "amd64"),
			"curl":  curlStub,
		}},
	)

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	if !strings.Contains(result.stdout, "no codecov CLI binary for FreeBSD/amd64") {
		t.Errorf("expected an explicit unsupported-platform reason, got: %s", result.stdout)
	}
	if _, err := os.Stat(curlArgsFile); !os.IsNotExist(err) {
		t.Errorf("an unsupported platform must not download anything")
	}
}

// TestUnusableCachedCLIIsDownloadedAgain verifies a cache entry that is
// truncated or built for another platform is replaced instead of being reused
// forever, which would keep uploads broken until TMPDIR is wiped.
func TestUnusableCachedCLIIsDownloadedAgain(t *testing.T) {
	cacheDir := t.TempDir()
	cached := filepath.Join(cacheDir, "codecovcli-"+scriptCLIVersion(t)+"-codecovcli_linux")
	if err := os.WriteFile(cached, []byte("truncated download\n"), 0o755); err != nil {
		t.Fatalf("failed to seed the cache: %v", err)
	}
	curlArgsFile := filepath.Join(t.TempDir(), "curl_args.txt")
	cliArgsFile := filepath.Join(t.TempDir(), "cli_args.txt")
	result := runCodecovScriptWithStubs(t,
		map[string]string{
			"TMPDIR":             cacheDir,
			"CURL_ARGS_FILE":     curlArgsFile,
			"FAKE_CLI_ARGS_FILE": cliArgsFile,
		},
		scriptStubs{withoutCLI: true, extra: map[string]string{
			"uname": unameStubFor("Linux", "x86_64"),
			"curl":  curlStub,
		}},
	)

	if result.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", result.exitCode, result.stdout)
	}
	if !strings.Contains(result.stdout, "unusable, re-downloading") {
		t.Errorf("expected the stale cache to be detected, got: %s", result.stdout)
	}
	curlArgs, err := os.ReadFile(curlArgsFile)
	if err != nil {
		t.Fatalf("the CLI was not downloaded again: %v", err)
	}
	if !strings.Contains(string(curlArgs), "codecovcli_linux") {
		t.Errorf("expected the Linux asset to be fetched, got %q", string(curlArgs))
	}
	cliArgs, err := os.ReadFile(cliArgsFile)
	if err != nil {
		t.Fatalf("the downloaded CLI was never invoked: %v", err)
	}
	if !strings.Contains(string(cliArgs), "upload-process") {
		t.Errorf("expected the downloaded CLI to receive the upload arguments, got %q", string(cliArgs))
	}
}

// pathWithoutCLI is the inherited PATH without any directory that provides a
// codecovcli, so tests that leave the stub off PATH cannot accidentally resolve
// the developer's own installation and skip the download path they test.
func pathWithoutCLI(t *testing.T) string {
	t.Helper()
	var kept []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "codecovcli")); err == nil {
			continue
		}
		kept = append(kept, dir)
	}
	return strings.Join(kept, string(os.PathListSeparator))
}

// TestPathWithoutCLIDropsOnlyDirectoriesWithTheCLI guards the isolation the
// withoutCLI tests rely on.
func TestPathWithoutCLIDropsOnlyDirectoriesWithTheCLI(t *testing.T) {
	cleanDir := t.TempDir()
	dirtyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirtyDir, "codecovcli"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to write codecovcli: %v", err)
	}
	t.Setenv("PATH", strings.Join([]string{cleanDir, dirtyDir}, string(os.PathListSeparator)))

	filtered := pathWithoutCLI(t)
	if strings.Contains(filtered, dirtyDir) {
		t.Errorf("PATH still contains %s, so an installed codecovcli stays resolvable: %s", dirtyDir, filtered)
	}
	if !strings.Contains(filtered, cleanDir) {
		t.Errorf("PATH no longer contains %s: %s", cleanDir, filtered)
	}
	for _, dir := range filepath.SplitList(filtered) {
		if _, err := os.Stat(filepath.Join(dir, "codecovcli")); err == nil {
			t.Errorf("codecovcli is still resolvable through %s: %s", dir, filtered)
		}
	}
}

// runTimeoutHelper runs hack/run_with_timeout.pl with the given arguments and
// returns its exit code and wall clock duration.
func runTimeoutHelper(t *testing.T, args ...string) (int, time.Duration) {
	t.Helper()
	return runTimeoutHelperWithEnv(t, nil, args...)
}

// runTimeoutHelperWithEnv is runTimeoutHelper with extra environment entries,
// used to make the helper run under a perl without Time::HiRes.
func runTimeoutHelperWithEnv(t *testing.T, env []string, args ...string) (int, time.Duration) {
	t.Helper()
	perl, err := exec.LookPath("perl")
	if err != nil {
		t.Skip("perl is not installed, so the timeout fallback cannot be exercised")
	}
	cmd := exec.Command(perl, append([]string{"run_with_timeout.pl"}, args...)...) // #nosec G204
	cmd.Env = append(os.Environ(), env...)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start)
	code := 0
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run run_with_timeout.pl: %v", err)
		}
		code = exitError.ExitCode()
	}
	t.Logf("run_with_timeout.pl %v -> %d in %s: %s", args, code, elapsed, stdout.String())
	return code, elapsed
}

// TestTimeoutHelperTreatsZeroAsNoDeadline keeps GNU timeout's `0` semantics:
// the command runs to completion rather than being killed or rejected.
func TestTimeoutHelperTreatsZeroAsNoDeadline(t *testing.T) {
	code, _ := runTimeoutHelper(t, "0", "bash", "-c", "sleep 1; exit 3")
	if code != 3 {
		t.Errorf("expected the command's exit code 3 when the deadline is disabled, got %d", code)
	}
}

// TestTimeoutHelperAcceptsFractionalDeadline keeps GNU timeout's fractional
// durations: 0.5 must expire instead of being rejected as malformed.
func TestTimeoutHelperAcceptsFractionalDeadline(t *testing.T) {
	code, elapsed := runTimeoutHelper(t, "0.5", "bash", "-c", "sleep 30")
	if code != 124 {
		t.Errorf("expected exit 124 when a 0.5s deadline expires, got %d", code)
	}
	if elapsed > 5*time.Second {
		t.Errorf("a 0.5s deadline took %s", elapsed)
	}
}

// TestTimeoutHelperKeepsDeadlineWithoutTimeHiRes covers the prow unit image: it
// runs a RHEL perl where Time::HiRes is a separate package that is not
// installed, so the helper must still enforce the deadline instead of dying at
// compile time and letting the upload run unbounded.
func TestTimeoutHelperKeepsDeadlineWithoutTimeHiRes(t *testing.T) {
	shadow := t.TempDir()
	if err := os.MkdirAll(filepath.Join(shadow, "Time"), 0o750); err != nil {
		t.Fatalf("failed to create the module shadow directory: %v", err)
	}
	stub := []byte("die \"Time::HiRes unavailable\\n\";\n1;\n")
	if err := os.WriteFile(filepath.Join(shadow, "Time", "HiRes.pm"), stub, 0o600); err != nil {
		t.Fatalf("failed to shadow Time::HiRes: %v", err)
	}

	code, elapsed := runTimeoutHelperWithEnv(t, []string{"PERL5LIB=" + shadow}, "0.5", "bash", "-c", "sleep 30")
	if code != 124 {
		t.Errorf("expected exit 124 when a 0.5s deadline expires without Time::HiRes, got %d", code)
	}
	if elapsed > 5*time.Second {
		t.Errorf("a 0.5s deadline without Time::HiRes took %s", elapsed)
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
