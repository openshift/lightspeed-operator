"""Offline integration tests for concurrent regional capacity searches."""

import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch

import deploy_cuda_x86_64 as deploy

DIRECTORY = Path(__file__).resolve().parent
REGIONS = ("us-east-1", "us-east-2", "us-west-1", "us-west-2")

# Every mock invocation is a separate process, just like the real AWS CLI.
# A locked state file makes assertions reliable when regional workers overlap.
AWS_MOCK = r"""#!/usr/bin/env python3
import fcntl
import json
import os
from pathlib import Path
import sys
import time

root = Path(os.environ['MOCK_ROOT'])
scenario = os.environ['MOCK_SCENARIO']
args = sys.argv[1:]
service, operation = args[:2]
def option(name):
    return args[args.index(name) + 1] if name in args else ''
region = option('--region')
stack = option('--stack-name')
query = option('--query')
result = ''
status = 0
with (root / 'lock').open('a') as lock:
    fcntl.flock(lock, fcntl.LOCK_EX)
    state_path = root / 'state.json'
    state = json.loads(state_path.read_text()) if state_path.exists() else {'counts': {}, 'polls': {}, 'created': [], 'calls': []}
    if operation == 'describe-instance-type-offerings':
        single = scenario in ('capacity-retry', 'poll-timeout', 'rollback-timeout', 'mixed-failure', 'status-failure', 'events-failure')
        if scenario == 'no-offering' or (single and region != 'us-east-1'):
            result = 'None'
        elif scenario == 'discovery-failure':
            status = 1
        else:
            result = f'{region}a\t{region}b'
    elif operation == 'describe-key-pairs':
        if 'PublicKey' in query:
            if scenario == 'source-key-failure': status = 1
            elif scenario == 'source-key-empty': result = 'None'
            else: result = 'ssh-rsa AAAAtest-key'
        elif scenario == 'key-query-failure':
            status = 1
        else:
            result = '0' if scenario in ('import-keys', 'source-key-failure', 'source-key-empty', 'import-failure') and region != 'us-west-2' else '1'
    elif operation == 'import-key-pair':
        material = Path(option('--public-key-material').removeprefix('fileb://')).read_text().strip()
        assert material == 'ssh-rsa AAAAtest-key'
        assert option('--key-name') == 'test-user-keys'
        result = 'test-user-keys'
        if scenario == 'import-failure': status = 1
    elif operation == 'create-stack':
        if scenario == 'existing-stack' and region == 'us-west-2':
            status = 1
        else:
            attempt = state['counts'].get(region, 0) + 1
            state['counts'][region] = attempt
            result = f'{region}-stack-{attempt}'
            state['created'].append(result)
            if scenario == 'missing-stack-id': result = 'None'
    elif operation == 'describe-stacks':
        assert stack in state['created'], (stack, state['created'])
        assert stack.startswith(region + '-stack-'), (region, stack)
        if 'StackStatus' in query:
            polls = state['polls'].get(stack, 0)
            state['polls'][stack] = polls + 1
            if scenario == 'status-failure': status = 1
            elif scenario == 'poll-timeout': result = 'CREATE_IN_PROGRESS'
            elif scenario == 'rollback-timeout': result = 'ROLLBACK_IN_PROGRESS'
            elif scenario in ('all-capacity', 'mixed-failure', 'events-failure'):
                result = 'ROLLBACK_COMPLETE'
            elif scenario == 'capacity-retry' and stack.endswith('-1'):
                result = 'ROLLBACK_IN_PROGRESS' if polls < 2 else 'ROLLBACK_COMPLETE'
            elif scenario == 'delayed-worker' and region == 'us-west-2' and not any(c['operation'] == 'describe-stacks' and 'Outputs' in c['query'] for c in state['calls']):
                result = 'CREATE_IN_PROGRESS'
            elif scenario in ('interrupt', 'group-interrupt') and polls < 8:
                result = 'CREATE_IN_PROGRESS'
            else: result = 'CREATE_COMPLETE'
        else:
            assert 'Outputs' in query
            result = 'instance outputs for ' + stack
            if scenario == 'output-failure': status = 1
            if scenario == 'backpressure': result += 'x' * 262144
    elif operation == 'describe-stack-events':
        if scenario == 'events-failure': status = 1
        elif "LogicalResourceId!='EC2InstanceX8664'" in query:
            result = 'AccessDenied creating Route' if scenario == 'mixed-failure' else 'None'
        else:
            result = 'We currently do not have sufficient g6.2xlarge capacity in the Availability Zone you requested'
    elif operation == 'delete-stack':
        assert stack in state['created'], 'Must never delete a pre-existing/untracked stack'
        assert stack.startswith(region + '-stack-')
        if scenario == 'cleanup-failure': status = 1
    elif operation == 'wait':
        assert args[2] == 'stack-delete-complete', 'Slow creation/rollback waiter used'
        if scenario == 'delete-wait-failure': status = 1
    else:
        raise AssertionError(args)
    state['calls'].append({'operation': operation, 'region': region, 'stack': stack, 'query': query, 'args': args, 'result': result})
    state_path.write_text(json.dumps(state))

# The creation barrier requires all four regions to start before any returns.
# Sequential implementations fail instead of silently passing concurrency tests.
if operation == 'create-stack' and status == 0 and scenario in ('parallel', 'delayed-worker', 'cleanup-failure', 'delete-wait-failure', 'interrupt', 'import-keys', 'output-failure', 'group-interrupt', 'closed-output', 'backpressure'):
    deadline = time.monotonic() + 4
    while time.monotonic() < deadline:
        with (root / 'lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            state = json.loads((root / 'state.json').read_text())
            if len(state['counts']) == 4: break
        time.sleep(0.01)
    else:
        print('Creation barrier timed out: regions are not concurrent', file=sys.stderr)
        sys.exit(1)
if operation == 'create-stack' and scenario in ('group-interrupt', 'closed-output'):
    time.sleep(0.3)  # Keep accepted creates in flight until the test cancels/closes output.
print(result)
sys.exit(status)
"""


class OutputModeTests(unittest.TestCase):
    def setUp(self):
        self.stdout_read, self.stdout_write = os.pipe()
        self.stderr_read, self.stderr_write = os.pipe()
        for fd in (self.stdout_read, self.stdout_write, self.stderr_read, self.stderr_write):
            self.addCleanup(os.close, fd)
        self.addCleanup(os.set_blocking, self.stdout_write, True)
        self.addCleanup(os.set_blocking, self.stderr_write, True)
        for target, fd in (("stdout", self.stdout_write), ("stderr", self.stderr_write)):
            stream = self.enterContext(patch.object(deploy.sys, target))
            stream.fileno.return_value = fd
        self.enterContext(patch.dict(os.environ, {"USERNAME": "test-user"}))
        self.enterContext(patch.object(deploy.shutil, "which", return_value="aws"))

    def test_main_restores_original_modes_on_every_exit(self):
        for outcome in (0, 1, deploy.DeploymentError("failed"), OSError("failed"), RuntimeError("failed")):
            with self.subTest(outcome=outcome):
                os.set_blocking(self.stderr_write, False)

                def run(_deployment):
                    self.assertFalse(os.get_blocking(self.stdout_write))
                    self.assertFalse(os.get_blocking(self.stderr_write))
                    if isinstance(outcome, Exception):
                        raise outcome
                    return outcome

                with patch.object(deploy.sys, "argv", ["deploy"]), patch.object(
                    deploy.signal, "signal"
                ), patch.object(deploy.Deployment, "run", run):
                    if isinstance(outcome, RuntimeError) and not isinstance(outcome, deploy.DeploymentError):
                        with self.assertRaises(RuntimeError):
                            deploy.main()
                    else:
                        self.assertEqual(deploy.main(), outcome if isinstance(outcome, int) else 1)
                self.assertTrue(os.get_blocking(self.stdout_write))
                self.assertFalse(os.get_blocking(self.stderr_write))

    def test_regular_file_output_stays_blocking(self):
        with tempfile.TemporaryFile() as output:
            deploy.sys.stdout.fileno.return_value = output.fileno()
            deployment = deploy.Deployment()
            try:
                self.assertTrue(os.get_blocking(output.fileno()))
                self.assertFalse(os.get_blocking(self.stderr_write))
            finally:
                os.set_blocking(self.stderr_write, True)

    def test_partial_setup_failure_restores_stdout(self):
        set_blocking = os.set_blocking

        def fail_stderr(fd, blocking):
            if fd == self.stderr_write and not blocking:
                raise OSError("setup failed")
            set_blocking(fd, blocking)

        with patch.object(deploy.os, "set_blocking", side_effect=fail_stderr):
            with self.assertRaises(OSError):
                deploy.Deployment()
        self.assertTrue(os.get_blocking(self.stdout_write))


class ParallelDeploymentTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        binary = self.root / "bin"
        binary.mkdir()
        (binary / "aws").write_text(AWS_MOCK)
        (binary / "curl").write_text(
            "#!/usr/bin/env bash\n"
            "[[ $MOCK_SCENARIO != ip-failure ]] || exit 1\n"
            "printf '%s\\n' \"${MOCK_IPV4-198.51.100.42}\"\n"
        )
        (binary / "sleep").write_text("#!/usr/bin/env bash\nexec /bin/sleep 0.02\n")
        for file in binary.iterdir():
            file.chmod(0o755)
        self.environment = {
            **os.environ,
            "PATH": f"{binary}:{os.environ['PATH']}",
            "MOCK_ROOT": str(self.root),
            "AWS_REGION": "us-west-2",
            "USERNAME": "test-user",
            "MY_IP_CIDR": "203.0.113.1/32",
            "STACK_POLL_INTERVAL_SECONDS": "0.02",
            "STACK_CREATE_MAX_POLLS": "30",
            "STACK_ROLLBACK_MAX_POLLS": "3",
        }

    def run_script(self, scenario, **overrides):
        environment = {**self.environment, "MOCK_SCENARIO": scenario, **overrides}
        environment = {
            key: value for key, value in environment.items() if value is not None
        }
        result = subprocess.run(
            ["bash", str(DIRECTORY / "deploy_cuda_x86_64.sh")],
            cwd=self.root,
            env=environment,
            capture_output=True,
            text=True,
            timeout=20,
        )
        state_path = self.root / "state.json"
        self.calls = (
            json.loads(state_path.read_text())["calls"] if state_path.exists() else []
        )
        return result

    def calls_for(self, operation):
        return [call for call in self.calls if call["operation"] == operation]

    def assert_single_winner(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        outputs = [
            call
            for call in self.calls_for("describe-stacks")
            if "Outputs" in call["query"]
        ]
        self.assertEqual(len(outputs), 1, self.calls)
        winner = outputs[0]["stack"]
        created = {call["result"] for call in self.calls_for("create-stack")}
        deleted = {call["stack"] for call in self.calls_for("delete-stack")}
        self.assertEqual(deleted, created - {winner})
        self.assertNotIn(winner, deleted)
        self.assertIn("instance outputs", result.stdout)
        return winner

    def test_regions_are_concurrent_and_only_one_success_is_retained(self):
        result = self.run_script("parallel")
        self.assert_single_winner(result)
        self.assertEqual(
            {call["region"] for call in self.calls_for("create-stack")}, set(REGIONS)
        )
        self.assertEqual(len(self.calls_for("create-stack")), 4)
        self.assertEqual(len(self.calls_for("delete-stack")), 3)

    def test_slow_inflight_loser_is_cleaned_without_becoming_a_second_winner(self):
        result = self.run_script("delayed-worker")
        winner = self.assert_single_winner(result)
        self.assertFalse(winner.startswith("us-west-2"))
        self.assertIn(
            "us-west-2-stack-1",
            {call["stack"] for call in self.calls_for("delete-stack")},
        )

    def test_keys_are_imported_before_any_worker_creates_a_stack(self):
        result = self.run_script("import-keys")
        self.assert_single_winner(result)
        imports = self.calls_for("import-key-pair")
        self.assertEqual(
            {call["region"] for call in imports}, set(REGIONS) - {"us-west-2"}
        )
        first_create = next(
            i
            for i, call in enumerate(self.calls)
            if call["operation"] == "create-stack"
        )
        self.assertTrue(all(self.calls.index(call) < first_create for call in imports))
        public_queries = [call for call in self.calls if "PublicKey" in call["query"]]
        self.assertEqual(len(public_queries), 1)
        for call in imports:
            material = call["args"][call["args"].index("--public-key-material") + 1]
            self.assertFalse(Path(material.removeprefix("fileb://")).exists())

    def test_capacity_shortage_retries_az_only_after_cleanup(self):
        result = self.run_script("capacity-retry")
        self.assertEqual(self.assert_single_winner(result), "us-east-1-stack-2")
        first_delete_wait = next(
            i for i, call in enumerate(self.calls) if call["operation"] == "wait"
        )
        second_create = [
            i
            for i, call in enumerate(self.calls)
            if call["operation"] == "create-stack"
        ][1]
        self.assertLess(first_delete_wait, second_create)

    def test_all_capacity_failures_leave_no_stacks(self):
        result = self.run_script("all-capacity")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(self.calls_for("create-stack")), 8)
        self.assertEqual(len(self.calls_for("delete-stack")), 8)
        self.assertFalse(any("Outputs" in call["query"] for call in self.calls))

    def test_help_and_unknown_arguments_never_deploy(self):
        for argument, expected_status in (("--help", 0), ("--unknown", 2)):
            with self.subTest(argument=argument):
                result = subprocess.run(
                    ["bash", str(DIRECTORY / "deploy_cuda_x86_64.sh"), argument],
                    cwd=self.root,
                    env={**self.environment, "MOCK_SCENARIO": "parallel"},
                    capture_output=True,
                    text=True,
                    timeout=10,
                )
                self.assertEqual(result.returncode, expected_status)
                self.assertIn("usage:", result.stdout + result.stderr)
                self.assertFalse((self.root / "state.json").exists())

    def test_no_offering_creates_nothing(self):
        result = self.run_script("no-offering")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.calls_for("create-stack"))

    def test_preflight_errors_start_no_workers(self):
        for scenario in (
            "discovery-failure",
            "key-query-failure",
            "source-key-failure",
            "source-key-empty",
            "import-failure",
        ):
            with self.subTest(scenario=scenario):
                state = self.root / "state.json"
                if state.exists():
                    state.unlink()
                result = self.run_script(scenario)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.calls_for("create-stack"))

    def test_existing_stack_is_never_deleted(self):
        result = self.run_script("existing-stack")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(
            any(
                call["stack"] == "test-user-cuda-x86-64-stack"
                for call in self.calls_for("delete-stack")
            )
        )

    def test_loser_cleanup_failure_is_reported_even_with_a_winner(self):
        for scenario in ("cleanup-failure", "delete-wait-failure"):
            with self.subTest(scenario=scenario):
                state = self.root / "state.json"
                if state.exists():
                    state.unlink()
                result = self.run_script(scenario)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(
                    len([call for call in self.calls if "Outputs" in call["query"]]), 1
                )
                self.assertIn("Manual cleanup", result.stdout + result.stderr)

    def test_mixed_failure_is_not_retried(self):
        result = self.run_script("mixed-failure")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(self.calls_for("create-stack")), 1)
        self.assertIn("Another resource failed", result.stdout + result.stderr)

    def test_poll_errors_and_timeouts_report_owned_stacks(self):
        for scenario in (
            "poll-timeout",
            "rollback-timeout",
            "status-failure",
            "events-failure",
        ):
            with self.subTest(scenario=scenario):
                state = self.root / "state.json"
                if state.exists():
                    state.unlink()
                result = self.run_script(scenario, STACK_CREATE_MAX_POLLS="3")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(len(self.calls_for("create-stack")), 1)

    def test_empty_create_response_is_not_deleted_by_name(self):
        result = self.run_script("missing-stack-id")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.calls_for("delete-stack"))
        self.assertIn("stack ID", result.stdout + result.stderr)

    def test_public_ip_is_optional(self):
        result = self.run_script("parallel", MY_IP_CIDR=None)
        self.assert_single_winner(result)
        for call in self.calls_for("create-stack"):
            self.assertIn(
                "ParameterKey=AllowedIngressCidr,ParameterValue=198.51.100.42/32",
                call["args"],
            )

    def test_ingress_cidr_is_normalized_before_stack_creation(self):
        result = self.run_script("parallel", MY_IP_CIDR="203.0.113.42/24")
        self.assert_single_winner(result)
        for call in self.calls_for("create-stack"):
            self.assertIn(
                "ParameterKey=AllowedIngressCidr,ParameterValue=203.0.113.0/24",
                call["args"],
            )

    def test_invalid_public_ip_and_ingress_are_rejected_before_aws(self):
        for address in ("", "not an IP", "999.0.0.1", "2001:db8::1", "01.2.3.4"):
            with self.subTest(address=address):
                result = self.run_script("parallel", MY_IP_CIDR=None, MOCK_IPV4=address)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.calls)
        for cidr in ("198.51.100.7/0", "999.0.0.1/32", "198.51.100.7/33", "::1/128"):
            with self.subTest(cidr=cidr):
                result = self.run_script("parallel", MY_IP_CIDR=cidr)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.calls)

    def test_output_failure_does_not_delete_the_selected_winner(self):
        result = self.run_script("output-failure")
        self.assertNotEqual(result.returncode, 0)
        outputs = [call for call in self.calls if "Outputs" in call["query"]]
        self.assertEqual(len(outputs), 1)
        self.assertNotIn(
            outputs[0]["stack"],
            {call["stack"] for call in self.calls_for("delete-stack")},
        )
        self.assertIn("Retained winner", result.stdout)

    def test_bad_inputs_and_ip_detection_do_not_call_aws(self):
        for overrides in (
            {"USERNAME": None},
            {"AWS_REGION": "eu-central-1"},
            {"STACK_POLL_INTERVAL_SECONDS": "0"},
            {"STACK_CREATE_MAX_POLLS": "bad"},
            {"STACK_ROLLBACK_MAX_POLLS": "0"},
        ):
            with self.subTest(overrides=overrides):
                result = self.run_script("parallel", **overrides)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.calls)
        result = self.run_script("ip-failure", MY_IP_CIDR=None)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.calls)

    def start_pending_workers(self, scenario, *, new_session=False):
        environment = {**self.environment, "MOCK_SCENARIO": scenario}
        process = subprocess.Popen(
            ["bash", str(DIRECTORY / "deploy_cuda_x86_64.sh")],
            cwd=self.root,
            env=environment,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            start_new_session=new_session,
        )
        self.addCleanup(lambda: process.kill() if process.poll() is None else None)
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            try:
                state = json.loads((self.root / "state.json").read_text())
                if len(state["counts"]) == 4:
                    return process
            except (FileNotFoundError, json.JSONDecodeError):
                pass
            time.sleep(0.01)
        self.fail("Workers never started")

    def test_terminal_sigint_does_not_kill_inflight_create_requests(self):
        process = self.start_pending_workers("group-interrupt", new_session=True)
        os.killpg(process.pid, signal.SIGINT)
        stdout, stderr = process.communicate(timeout=10)
        self.assertNotEqual(process.returncode, 0, stdout + stderr)
        calls = json.loads((self.root / "state.json").read_text())["calls"]
        self.assertEqual(
            len([call for call in calls if call["operation"] == "delete-stack"]), 4
        )
        self.assertFalse(any("Outputs" in call["query"] for call in calls))

    def test_undrained_stdout_pipe_does_not_block_loser_cleanup(self):
        process = self.start_pending_workers("backpressure")
        try:
            process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()
            process.communicate()
            self.fail("Cleanup blocked by stdout backpressure")
        stdout, stderr = process.communicate()
        self.assertNotEqual(process.returncode, 0, stdout + stderr)
        calls = json.loads((self.root / "state.json").read_text())["calls"]
        winners = {call["stack"] for call in calls if "Outputs" in call["query"]}
        created = {
            call["result"] for call in calls if call["operation"] == "create-stack"
        }
        deleted = {
            call["stack"] for call in calls if call["operation"] == "delete-stack"
        }
        self.assertEqual(len(winners), 1)
        self.assertEqual(deleted, created - winners)

    def test_closed_stdout_does_not_prevent_loser_cleanup(self):
        process = self.start_pending_workers("closed-output")
        process.stdout.close()
        process.stdout = None
        _, stderr = process.communicate(timeout=10)
        self.assertNotEqual(process.returncode, 0, stderr)
        calls = json.loads((self.root / "state.json").read_text())["calls"]
        created = {
            call["result"] for call in calls if call["operation"] == "create-stack"
        }
        deleted = {
            call["stack"] for call in calls if call["operation"] == "delete-stack"
        }
        winners = {call["stack"] for call in calls if "Outputs" in call["query"]}
        self.assertLessEqual(len(winners), 1)
        self.assertEqual(deleted, created - winners)

    def test_sigterm_cooperatively_cleans_workers(self):
        environment = {**self.environment, "MOCK_SCENARIO": "interrupt"}
        process = subprocess.Popen(
            ["bash", str(DIRECTORY / "deploy_cuda_x86_64.sh")],
            cwd=self.root,
            env=environment,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        self.addCleanup(lambda: process.kill() if process.poll() is None else None)
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            try:
                state = json.loads((self.root / "state.json").read_text())
                if len(state["counts"]) == 4:
                    break
            except (FileNotFoundError, json.JSONDecodeError):
                pass
            time.sleep(0.01)
        else:
            self.fail("Workers never started")
        process.send_signal(signal.SIGTERM)
        stdout, stderr = process.communicate(timeout=10)
        self.assertNotEqual(process.returncode, 0, stdout + stderr)
        calls = json.loads((self.root / "state.json").read_text())["calls"]
        self.assertEqual(
            len([call for call in calls if call["operation"] == "delete-stack"]), 4
        )
        self.assertFalse(any("Outputs" in call["query"] for call in calls))


if __name__ == "__main__":
    unittest.main()
