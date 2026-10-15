#!/usr/bin/env python3
"""Run with python3; requires PyYAML. No registry or cluster access is used."""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[1]
ACTION = ROOT / "stepactions/secure-push-oci/0.1/secure-push-oci.yaml"
PIPELINES = [
    "lightspeed-operator-e2e-test-pipeline.yaml",
    "lightspeed-operator-e2e-test-all-features.yaml",
    "lightspeed-operator-upgrade-e2e-test-pipeline.yaml",
    "lightspeed-service-integration-test-pipeline.yaml",
]


class PipelineTests(unittest.TestCase):
    def test_publishers_are_run_specific_bounded_and_pinned_to_tested_commit(self):
        for filename in PIPELINES:
            with self.subTest(pipeline=filename):
                pipeline = yaml.safe_load((ROOT / "pipelines" / filename).read_text())
                task = next(t for t in pipeline["spec"]["tasks"]
                            if any(s["name"] == "push-artifacts"
                                   for s in t.get("taskSpec", {}).get("steps", [])))
                task_params = {p["name"]: p["value"] for p in task["params"]}
                self.assertEqual(task_params.get("artifact-run-name"), "$(context.pipelineRun.name)")
                self.assertIn("artifact-run-name", [p["name"] for p in task["taskSpec"]["params"]])
                steps = task["taskSpec"]["steps"]
                publish = next(s for s in steps if s["name"] == "push-artifacts")
                ref = {p["name"]: p["value"] for p in publish["ref"]["params"]}
                self.assertEqual(ref["url"], "https://github.com/openshift/lightspeed-operator")
                self.assertEqual(ref["revision"], "$(params.commit)")
                self.assertEqual(ref["pathInRepo"], str(ACTION.relative_to(ROOT.parents[1])))
                params = {p["name"]: p["value"] for p in publish["params"]}
                self.assertIn(":$(params.artifact-run-name)-", params["oci-ref"])
                self.assertTrue(params["oci-ref"].endswith("$(params.commit)"))
                self.assertEqual(publish.get("timeout"), "15m")
                self.assertEqual(publish["onError"], "continue")
                self.assertEqual(steps[-1]["name"], "fail-if-any-step-failed")

    def test_tags_distinguish_bundle_direct_and_retries_for_the_same_commit(self):
        pipeline = yaml.safe_load((ROOT / "pipelines" / PIPELINES[0]).read_text())
        task = next(t for t in pipeline["spec"]["tasks"] if t["name"] == "ols-operator-tests")
        publish = next(s for s in task["taskSpec"]["steps"] if s["name"] == "push-artifacts")
        template = next(p["value"] for p in publish["params"] if p["name"] == "oci-ref")
        refs = []
        for run in ["bundle-e2e-tests-first", "operator-e2e-tests-direct-first", "bundle-e2e-tests-retry"]:
            ref = template.replace("$(params.artifact-run-name)", run)
            ref = ref.replace("$(params.artifact-oci-tag-prefix)", "419")
            ref = ref.replace("$(params.commit)", "a" * 40)
            self.assertNotIn("$(", ref)
            self.assertLessEqual(len(ref.rsplit(":", 1)[1]), 128)
            refs.append(ref)
        self.assertEqual(len(set(refs)), 3)


class PublisherTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue(ACTION.exists(), "repository-owned publisher StepAction is missing")
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.work = self.base / "work"
        self.work.mkdir()
        (self.work / "data.txt").write_text("fresh diagnostics\n")
        self.bin = self.base / "bin"
        self.bin.mkdir()
        self.calls = self.base / "calls.jsonl"
        self.calls.touch()
        self.env = dict(os.environ, PATH=f"{self.bin}:{os.environ['PATH']}",
                        CALLS=str(self.calls), OCI_ARTIFACT_REFERENCE="quay.io/example/artifacts:bundle-run-abc123",
                        OCI_TAG_EXPIRATION="1y", SCAN_TIMEOUT="1s", PUSH_TIMEOUT="1s",
                        RETRY_DELAY="0", TMPDIR=str(self.base))
        self.command("leaktk-scanner", '''import os, sys, time
if os.environ.get("PATTERN_FETCH_STARTED"):
    print("[INFO] fetching gitleaks patterns", file=sys.stderr, flush=True)
    print("synthetic-sensitive-value", file=sys.stderr, flush=True)
if os.environ.get("PATTERN_FETCH_COMPLETED"):
    print("[INFO] updated gitleaks patterns: hash=synthetic", file=sys.stderr, flush=True)
if os.environ.get("SCAN_HANG"): time.sleep(30)
print("synthetic-sensitive-value")
sys.exit(int(os.environ.get("SCAN_EXIT", "0")))
''')
        self.command("leaktk-remove-files", '''import os, sys
sys.stdin.read()
sys.exit(int(os.environ.get("REMOVE_EXIT", "0")))
''')
        self.command("oras", '''import json, os, pathlib, sys, time
with open(os.environ["CALLS"], "a") as f:
    f.write(json.dumps({"args": sys.argv[1:], "files": sorted(p.name for p in pathlib.Path.cwd().iterdir())}) + "\\n")
if os.environ.get("PUSH_HANG"): time.sleep(30)
state = pathlib.Path(os.environ["CALLS"] + ".attempts")
n = int(state.read_text()) + 1 if state.exists() else 1
state.write_text(str(n))
if n <= int(os.environ.get("PUSH_FAILURES", "0")): sys.exit(11)
print("Pushed synthetic artifact")
''')
        self.script = self.base / "publisher.sh"
        self.script.write_text(yaml.safe_load(ACTION.read_text())["spec"]["script"])

    def command(self, name, source):
        path = self.bin / name
        path.write_text("#!/usr/bin/env python3\n" + source)
        path.chmod(0o755)

    def run_publisher(self, **env):
        result = subprocess.run(["bash", str(self.script)], cwd=self.work,
                                env=dict(self.env, **env), capture_output=True, text=True, timeout=15)
        self.assertNotIn("synthetic-sensitive-value", result.stdout + result.stderr)
        calls = [json.loads(line) for line in self.calls.read_text().splitlines()]
        return result, calls

    def test_success_scans_then_uploads_without_downloading_old_files(self):
        result, calls = self.run_publisher()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0]["args"][0], "push")
        self.assertEqual(calls[0]["files"], ["data.txt"])
        self.assertIn("[scan] starting", result.stdout)
        self.assertIn("[scan] completed", result.stdout)
        self.assertIn("[upload] completed", result.stdout)
        self.assertEqual(sorted(p.name for p in self.base.iterdir()),
                         ["bin", "calls.jsonl", "calls.jsonl.attempts", "publisher.sh", "work"])

    def test_failed_scan_never_uploads(self):
        result, calls = self.run_publisher(SCAN_EXIT="7")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])
        self.assertIn("[scan] failed", result.stdout)

    def test_failed_removal_never_uploads(self):
        result, calls = self.run_publisher(REMOVE_EXIT="8")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])

    def test_scan_timeout_never_uploads(self):
        result, calls = self.run_publisher(SCAN_HANG="1")
        self.assertEqual(result.returncode, 124)
        self.assertEqual(calls, [])
        self.assertIn("[scan] failed", result.stdout)
        self.assertIn("124", result.stdout)

    def test_scan_timeout_reports_only_safe_pattern_fetch_progress(self):
        result, calls = self.run_publisher(SCAN_HANG="1", PATTERN_FETCH_STARTED="1")
        self.assertEqual(result.returncode, 124)
        self.assertEqual(calls, [])
        self.assertIn("[scan] pattern fetch started; no completion marker", result.stdout)

    def test_scan_failure_distinguishes_completed_pattern_fetch(self):
        result, calls = self.run_publisher(SCAN_EXIT="7", PATTERN_FETCH_STARTED="1", PATTERN_FETCH_COMPLETED="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])
        self.assertIn("[scan] pattern fetch completion marker observed before failure", result.stdout)
        self.assertNotIn("no completion marker", result.stdout)

    def test_upload_retries_transient_failures(self):
        result, calls = self.run_publisher(PUSH_FAILURES="2")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(len(calls), 3)

    def test_upload_retry_exhaustion_fails_the_step(self):
        result, calls = self.run_publisher(PUSH_FAILURES="9")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(calls), 3)
        self.assertIn("[upload] failed", result.stdout)

    def test_hung_uploads_are_bounded(self):
        result, calls = self.run_publisher(PUSH_HANG="1")
        self.assertEqual(result.returncode, 124)
        self.assertEqual(len(calls), 3)
        self.assertIn("[upload] failed", result.stdout)

    def test_invalid_reference_is_rejected(self):
        result, calls = self.run_publisher(OCI_ARTIFACT_REFERENCE="not-a-registry-reference")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])

    def test_oversized_tag_is_rejected_without_truncation(self):
        result, calls = self.run_publisher(OCI_ARTIFACT_REFERENCE="quay.io/example/artifacts:" + "a" * 129)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])

    def test_maximum_length_tag_is_accepted(self):
        result, calls = self.run_publisher(OCI_ARTIFACT_REFERENCE="quay.io/example/artifacts:" + "a" * 128)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(len(calls), 1)


if __name__ == "__main__":
    unittest.main()
