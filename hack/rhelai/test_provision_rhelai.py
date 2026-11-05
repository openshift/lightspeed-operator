"""Offline, subprocess-level tests: never invoke real AWS, SSH or Podman."""

import base64
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from test_deploy_cuda_x86_64 import AWS_MOCK, DIRECTORY

# Preserve the existing concurrent AWS simulator, supplying the additional read
# APIs and real, ephemeral public keys needed by the end-to-end provisioner.
PROVISION_AWS = (
    AWS_MOCK.replace("import time\n", "import time\nimport subprocess\n")
    .replace(
        "    if operation == 'describe-instance-type-offerings':",
        """    if operation == 'get-caller-identity':
        if scenario == 'expired-token': status = 1
        else: result = '123456789012'
    elif operation == 'describe-stack-resource':
        assert stack in state['created']
        result = 'i-0123456789abcdef0'
    elif operation == 'describe-instances':
        result = '198.51.100.123'
    elif operation == 'describe-instance-type-offerings':""",
    )
    .replace(
        "else: result = 'ssh-rsa AAAAtest-key'",
        "else: result = subprocess.check_output(['ssh-keygen', '-y', '-P', '', '-f', os.environ['MOCK_KEY_FILE']], text=True).strip() if scenario != 'key-mismatch' else 'ssh-rsa WRONGKEY'",
    )
    .replace(
        "result = '0' if scenario in ('import-keys', 'source-key-failure', 'source-key-empty', 'import-failure') and region != 'us-west-2' else '1'",
        "result = '0' if scenario == 'zero-keys' else '1'",
    )
    .replace(
        "assert material == 'ssh-rsa AAAAtest-key'",
        "assert material.split()[:2] == subprocess.check_output(['ssh-keygen', '-y', '-P', '', '-f', os.environ['MOCK_KEY_FILE']], text=True).split()[:2]",
    )
    .replace(
        "            result = 'instance outputs for ' + stack",
        """            result = 'instance outputs for ' + stack
            if option('--output') == 'json':
                result = json.dumps([
                    {'OutputKey': 'AWSRegion', 'OutputValue': region},
                    {'OutputKey': 'StackName', 'OutputValue': 'test-user-rhelai-stack'},
                    {'OutputKey': 'X8664PublicIp', 'OutputValue': '198.51.100.123'},
                    {'OutputKey': 'X8664InstanceId', 'OutputValue': 'i-0123456789abcdef0'},
                ])""",
    )
)

PROVISION_AWS = PROVISION_AWS.replace(
    "if scenario == 'status-failure': status = 1",
    "if stack in state.get('deleted', []): result = 'DELETE_COMPLETE'\n            elif scenario == 'status-failure': status = 1",
).replace(
    "if scenario == 'delete-wait-failure': status = 1",
    "if scenario == 'delete-wait-failure': status = 1\n        if status == 0: state.setdefault('deleted', []).append(stack)",
)

PROVISION_AWS = PROVISION_AWS.replace(
    "if scenario == 'cleanup-failure': status = 1",
    "if scenario in ('cleanup-failure', 'resume-both-fail'): status = 1",
)

# A losing deletion cannot finish until the winner has begun bootstrap. This
# barrier fails serialized implementations rather than relying on clock timing.
PROVISION_AWS = PROVISION_AWS.replace(
    "scenario in ('parallel', 'delayed-worker'",
    "scenario in ('parallel', 'cleanup-overlap', 'cleanup-overlap-failure', 'cleanup-overlap-bootstrap-failure', 'delayed-worker'",
).replace(
    "print(result)\nsys.exit(status)",
    """if operation == 'wait' and scenario in ('cleanup-overlap', 'cleanup-overlap-failure', 'cleanup-overlap-bootstrap-failure', 'resume-cleanup-overlap'):
    required = 'verify' if scenario == 'resume-cleanup-overlap' else 'convert'
    deadline = time.monotonic() + 4
    while time.monotonic() < deadline:
        path = root / 'remote.json'
        try:
            remote = json.loads(path.read_text()) if path.exists() else {'calls': []}
        except json.JSONDecodeError:
            remote = {'calls': []}
        if any(call.get('action') == required for call in remote['calls']):
            (root / ('cleanup-gate-' + region)).write_text(required)
            if scenario == 'cleanup-overlap-failure': status = 1
            break
        time.sleep(0.01)
    else:
        print('Cleanup barrier timed out: bootstrap waited for cleanup', file=sys.stderr)
        status = 1
print(result)
sys.exit(status)""",
)

SSH_MOCK = r"""#!/usr/bin/env python3
import ast
import json
import os
from pathlib import Path
import shlex
import sys

root = Path(os.environ['MOCK_ROOT'])
path = root / 'remote.json'
state = json.loads(path.read_text()) if path.exists() else {'calls': [], 'converted': False}
args = sys.argv[1:]
command = args[-1]
user = 'cloud-user' if 'cloud-user@' in ' '.join(args) else 'ec2-user'
scenario = os.environ['MOCK_SCENARIO']
payload = sys.stdin.buffer.read()
call = {'args': args, 'user': user}
status = 0
output = ''
if 'RHELAI_INSPECT' in command:
    call['action'] = 'inspect'
    if user == 'cloud-user' and not state['converted']:
        status = 255
    elif user == 'ec2-user' and state['converted'] and scenario not in ('convert-failure', 'cleanup-overlap-bootstrap-failure'):
        status = 255
    else:
        from_bootc = state['converted'] and scenario not in ('convert-failure', 'cleanup-overlap-bootstrap-failure')
        output = json.dumps({
            'instance_id': 'i-OTHER' if scenario == 'wrong-instance' else 'i-0123456789abcdef0',
            'boot_id': 'boot-new' if from_bootc else 'boot-old',
            'phase': 'failed' if scenario in ('convert-failure', 'cleanup-overlap-bootstrap-failure') and state['converted'] else 'configured' if state.get('configured') else 'rebooting' if state['converted'] else '',
            'bootc': {'status': {'booted': {'image': {
                'image': {'image': 'registry.redhat.io/rhelai3/bootc-cuda-rhel9@sha256:example' if scenario == 'digest-only-ref' else 'registry.redhat.io/rhelai3/bootc-cuda-rhel9:3.4.5-1790868254'},
                'imageDigest': 'sha256:e0fbb896811ab54d183e23612030072b946674824bad697bf2b3f68362651662' if scenario != 'wrong-image' else 'sha256:WRONG'
            }}}} if from_bootc else None,
        })
elif 'RHELAI_UPLOAD' in command:
    call['action'] = 'upload'
    call['size'] = len(payload)
    call['credential'] = command.endswith("'auth.json'") or '/auth.json' in command
    if call['credential']: state['auth_present'] = True
elif 'agent.sh configure' in command:
    call['action'] = 'configure'
    state['configured'] = True
    state['auth_present'] = False
    words = shlex.split(command)
    agent = next(i for i, word in enumerate(words) if word.endswith('/agent.sh'))
    settings = words[agent + 1:]
    state['model'] = settings[4] if len(settings) > 4 else 'ibm-granite/granite-3.3-2b-instruct'
    call['model'] = state['model']
    call['tensor_parallel_size'] = settings[5] if len(settings) > 5 else '1'
    call['max_num_seqs'] = settings[6] if len(settings) > 6 else '256'
elif 'systemd-run' in command:
    call['action'] = 'convert'
    state['converted'] = True
elif 'RHELAI_VERIFY' in command:
    assert state.get('configured'), 'Verification attempted without configuring the server'
    call['action'] = 'verify'
    code = shlex.split(command)[-1]
    call['model'] = ast.literal_eval(code.splitlines()[0].split('=', 1)[1])
    assert call['model'] == state['model'], 'Verification used a different model'
    if scenario in ('health-failure', 'resume-both-fail'): status = 1
    else:
        output = json.dumps({
            'models': {'object': 'list', 'data': [{'id': 'wrong-model' if scenario == 'wrong-model' else state['model'], 'object': 'model', 'owned_by': 'vllm'}]},
            'completion': {'object': 'text_completion', 'choices': [{'index': 0, 'text': 'The capital of France is Paris.' if scenario != 'empty-completion' else '', 'finish_reason': 'stop'}]},
        })
else:
    raise AssertionError(command)
host_error = False
if scenario == 'delayed-cloud-user' and state['converted'] and call['action'] == 'inspect':
    if user == 'ec2-user':
        status = 255
        host_error = True
    elif not state.get('delayed_probe'):
        state['delayed_probe'] = True
        status = 255
        output = ''
if scenario in ('early-host-key', 'unexpected-host-key'):
    hosts = Path(next(arg.split('=', 1)[1] for arg in args if arg.startswith('UserKnownHostsFile=')))
    if user == 'ec2-user':
        hosts.write_text('198.51.100.123 ssh-ed25519 OLDKEY\n')
    elif scenario == 'early-host-key' and not state.get('early_probe'):
        hosts.write_text('198.51.100.123 ssh-ed25519 OLDKEY\n')
        state['early_probe'] = True
        status = 255
    elif scenario == 'early-host-key' and hosts.exists() and 'OLDKEY' in hosts.read_text():
        status = 255
        host_error = True
    elif scenario == 'unexpected-host-key' and call['action'] == 'upload':
        status = 255
        host_error = True
    else:
        hosts.write_text('198.51.100.123 ssh-ed25519 NEWKEY\n')
state['calls'].append(call)
# Polling test readers must not observe a truncated half-written checkpoint.
temporary = root / ('remote.' + str(os.getpid()) + '.tmp')
temporary.write_text(json.dumps(state))
temporary.replace(path)
if scenario == 'ssh-stalls' and call['action'] == 'verify':
    import time
    time.sleep(30)
print(output)
if status: print('REMOTE HOST IDENTIFICATION HAS CHANGED' if host_error else 'mock transport/service failure', file=sys.stderr)
sys.exit(status)
"""


class ProvisioningTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        binary = self.root / "bin"
        binary.mkdir()
        (binary / "aws").write_text(PROVISION_AWS)
        (binary / "ssh").write_text(SSH_MOCK)
        (binary / "podman").write_text(
            "#!/usr/bin/env bash\n"
            "[[ $1 == manifest && $2 == inspect ]] || exit 9\n"
            "echo 'Error: Docker references with both a tag and digest are currently not supported' >&2\n"
            "exit 125\n"
        )
        (binary / "skopeo").write_text(
            "#!/usr/bin/env bash\n"
            "[[ $1 == inspect && $2 == --raw && $3 == --authfile ]] || exit 9\n"
            "[[ $5 == docker://registry.redhat.io/*@sha256:* ]] || exit 9\n"
            "[[ $5 != *:3.4.5*@sha256:* ]] || exit 125\n"
            "[[ $MOCK_SCENARIO != registry-denied ]] || exit 1\n"
            "if [[ $MOCK_SCENARIO == registry-local-error ]]; then\n"
            "  echo 'local inspection failed REGISTRY_SECRET_DO_NOT_PRINT' >&2; exit 125\n"
            "fi\n"
            'printf \'{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json"}\\n\'\n'
        )
        for file in binary.iterdir():
            file.chmod(0o755)
        self.key = self.root / "key.pem"
        subprocess.run(
            ["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(self.key)],
            check=True,
            capture_output=True,
        )
        self.auth = self.root / "auth.json"
        self.secret = "REGISTRY_SECRET_DO_NOT_PRINT"
        self.encoded_secret = base64.b64encode(f"user:{self.secret}".encode()).decode()
        self.auth.write_text(
            json.dumps({"auths": {"registry.redhat.io": {"auth": self.encoded_secret}}})
        )
        self.auth.chmod(0o600)
        self.state_dir = self.root / "provision"
        self.environment = {
            **os.environ,
            "PATH": f"{binary}:{os.environ['PATH']}",
            "MOCK_ROOT": str(self.root),
            "MOCK_KEY_FILE": str(self.key),
            "USERNAME": "test-user",
            "AWS_REGION": "us-west-2",
            "MY_IP_CIDR": "203.0.113.1/32",
            "STACK_POLL_INTERVAL_SECONDS": "0.02",
        }

    def run_script(
        self, scenario="parallel", extra=(), resume=False, default_auth=False
    ):
        args = [
            "bash",
            str(DIRECTORY / "provision_rhelai.sh"),
            "--timeout",
            "2",
            "--poll-interval",
            "0.01",
        ]
        if not default_auth:
            args += ["--registry-authfile", str(self.auth)]
        if resume:
            args += ["--resume", str(self.state_dir / "state.json")]
        else:
            args += ["--key-file", str(self.key), "--state-dir", str(self.state_dir)]
        result = subprocess.run(
            [*args, *extra],
            cwd=self.root,
            env={**self.environment, "MOCK_SCENARIO": scenario},
            capture_output=True,
            text=True,
            timeout=20,
        )
        self.result = result
        self.aws_state = self.load("state.json", {"calls": [], "created": []})
        self.remote = self.load("remote.json", {"calls": []})
        return result

    def load(self, name, default=None):
        path = self.root / name
        return json.loads(path.read_text()) if path.exists() else default

    def assert_no_secret(self):
        state = (self.state_dir / "state.json").read_text()
        transcript = (
            self.result.stdout
            + self.result.stderr
            + json.dumps(self.aws_state)
            + json.dumps(self.remote)
            + state
        )
        self.assertNotIn(self.secret, transcript)
        self.assertNotIn(self.encoded_secret, transcript)

    def test_zero_to_healthy_inference_retains_only_winner(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        state = self.load("provision/state.json")
        self.assertEqual(state["phase"], "ready")
        self.assertEqual(state["instance_id"], "i-0123456789abcdef0")
        self.assertEqual(len(state["created"]), 4)
        deleted = {
            c["stack"]
            for c in self.aws_state["calls"]
            if c["operation"] == "delete-stack"
        }
        self.assertEqual(
            deleted, set(self.aws_state["created"]) - {state["winner"]["stack_id"]}
        )
        self.assertTrue(self.remote["configured"])
        self.assertFalse(self.remote["auth_present"])
        self.assertIn("Paris", result.stdout)
        self.assertIn("cloud-user@198.51.100.123", result.stdout)
        self.assertIn("http://198.51.100.123:8000/v1", result.stdout)
        self.assert_no_secret()

    def test_default_startup_concurrency_is_16(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.load("provision/state.json")["max_num_seqs"], 16)
        configured = next(
            c for c in self.remote["calls"] if c.get("action") == "configure"
        )
        self.assertEqual(configured["max_num_seqs"], "16")

    def test_custom_concurrency_is_saved_and_restored_on_resume(self):
        first = self.run_script(extra=["--max-num-seqs", "32"])
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        self.assertEqual(self.load("provision/state.json")["max_num_seqs"], 32)
        remote = self.remote
        remote["configured"] = False
        (self.root / "remote.json").write_text(json.dumps(remote))
        resumed = self.run_script(resume=True)
        self.assertEqual(resumed.returncode, 0, resumed.stdout + resumed.stderr)
        configured = [c for c in self.remote["calls"] if c.get("action") == "configure"]
        self.assertEqual([c["max_num_seqs"] for c in configured], ["32", "32"])
        self.assertEqual(self.load("provision/state.json")["max_num_seqs"], 32)

    def test_invalid_saved_concurrency_is_rejected(self):
        from provision_rhelai import deployment_settings, DeploymentError

        for value in (0, -1, 4097, True, "16", None):
            with self.subTest(value=value):
                with self.assertRaises(DeploymentError):
                    deployment_settings({"max_num_seqs": value})

    def test_custom_instance_and_model_propagate_end_to_end_and_resume(self):
        model = "RedHatAI/gemma-3-12b-it"
        result = self.run_script(
            extra=[
                "--instance-type",
                "g5.12xlarge",
                "--model",
                model,
                "--tensor-parallel-size",
                "4",
            ]
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        state = self.load("provision/state.json")
        self.assertEqual(state["instance_type"], "g5.12xlarge")
        self.assertEqual(state["model"], model)
        self.assertEqual(state["tensor_parallel_size"], 4)
        for call in self.aws_state["calls"]:
            if call["operation"] == "describe-instance-type-offerings":
                self.assertIn("Name=instance-type,Values=g5.12xlarge", call["args"])
            if call["operation"] == "create-stack":
                self.assertIn(
                    "ParameterKey=InstanceType,ParameterValue=g5.12xlarge", call["args"]
                )
        configured = next(
            c for c in self.remote["calls"] if c.get("action") == "configure"
        )
        self.assertEqual(configured["model"], model)
        self.assertEqual(configured["tensor_parallel_size"], "4")
        self.assertIn("Model: " + model, result.stdout)
        resumed = self.run_script(resume=True)
        self.assertEqual(resumed.returncode, 0, resumed.stdout + resumed.stderr)
        self.assertEqual(self.remote["calls"][-1]["model"], model)
        self.assert_no_secret()

    def test_invalid_selection_is_rejected_before_aws(self):
        for extra in (
            ["--model", "bad\\nEnvironment=LEAK"],
            ["--instance-type", "g5g.16xlarge"],
            ["--tensor-parallel-size", "0"],
            ["--tensor-parallel-size", "-1"],
            ["--tensor-parallel-size", "65"],
            ["--max-num-seqs", "0"],
            ["--max-num-seqs", "-1"],
            ["--max-num-seqs", "4097"],
            ["--max-num-seqs", "not-a-number"],
        ):
            with self.subTest(extra=extra):
                result = self.run_script(extra=extra)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.aws_state["calls"])
                self.assertFalse(self.remote["calls"])

    def test_resume_cannot_override_deployment_selection(self):
        first = self.run_script()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        before = len(self.remote["calls"])
        for extra in (
            ["--model", "RedHatAI/gemma-3-12b-it"],
            ["--instance-type", "g5.12xlarge"],
            ["--tensor-parallel-size", "4"],
            ["--max-num-seqs", "32"],
        ):
            result = self.run_script(resume=True, extra=extra)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("--resume cannot be combined", result.stderr)
            self.assertEqual(len(self.remote["calls"]), before)

    def test_legacy_state_without_selection_uses_original_defaults(self):
        first = self.run_script()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        state = self.load("provision/state.json")
        for key in ("instance_type", "model", "tensor_parallel_size", "max_num_seqs"):
            state.pop(key, None)
        (self.state_dir / "state.json").write_text(json.dumps(state))
        resumed = self.run_script(resume=True)
        self.assertEqual(resumed.returncode, 0, resumed.stdout + resumed.stderr)
        self.assertEqual(self.load("provision/state.json")["max_num_seqs"], 256)

    def test_ssh_secrets_use_stdin_and_host_keys_are_phase_specific(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        calls = self.remote["calls"]
        credentials = [c for c in calls if c.get("credential")]
        self.assertTrue(credentials)
        self.assertTrue(all(c["size"] == self.auth.stat().st_size for c in credentials))
        for call in calls:
            self.assertIn("StrictHostKeyChecking=accept-new", " ".join(call["args"]))
            self.assertNotIn("StrictHostKeyChecking=no", " ".join(call["args"]))
            phase = "rhelai" if call["user"] == "cloud-user" else "rhel"
            self.assertIn(f"known_hosts.{phase}", " ".join(call["args"]))
        self.assert_no_secret()

    def test_creates_private_key_and_imports_all_regions_from_zero(self):
        self.key.unlink()
        self.key.with_suffix(".pem.pub").unlink()
        result = self.run_script("zero-keys")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.key.stat().st_mode & 0o777, 0o600)
        imports = [
            c for c in self.aws_state["calls"] if c["operation"] == "import-key-pair"
        ]
        self.assertEqual(len(imports), 4)
        first_create = next(
            i
            for i, c in enumerate(self.aws_state["calls"])
            if c["operation"] == "create-stack"
        )
        self.assertTrue(
            all(
                c["operation"] != "import-key-pair"
                for c in self.aws_state["calls"][first_create:]
            )
        )

    def test_uses_podman_runtime_auth_file_without_a_flag(self):
        runtime = self.root / "runtime"
        directory = runtime / "containers"
        directory.mkdir(parents=True)
        auth = directory / "auth.json"
        auth.write_bytes(self.auth.read_bytes())
        auth.chmod(0o600)
        self.environment.update(XDG_RUNTIME_DIR=str(runtime), REGISTRY_AUTH_FILE="")
        result = self.run_script(default_auth=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_no_secret()

    def test_honors_registry_auth_file_environment_override(self):
        self.environment.update(
            REGISTRY_AUTH_FILE=str(self.auth),
            XDG_RUNTIME_DIR=str(self.root / "missing"),
        )
        result = self.run_script(default_auth=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_no_secret()

    def test_explicit_auth_file_overrides_environment(self):
        self.environment["REGISTRY_AUTH_FILE"] = str(self.root / "missing.json")
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_missing_default_auth_file_explains_login(self):
        self.environment.update(
            XDG_RUNTIME_DIR=str(self.root / "missing"), REGISTRY_AUTH_FILE=""
        )
        result = self.run_script(default_auth=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("podman login", result.stderr)
        self.assertFalse(self.aws_state["created"])

    def test_preserves_existing_private_key(self):
        original = self.key.read_bytes()
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.key.read_bytes(), original)

    def test_does_not_overwrite_orphan_public_key(self):
        self.key.unlink()
        public = Path(str(self.key) + ".pub")
        original = public.read_bytes()
        result = self.run_script("zero-keys")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("public key", result.stderr.lower())
        self.assertEqual(public.read_bytes(), original)
        self.assertFalse(self.aws_state["created"])

    def test_mismatched_regional_key_stops_before_creating_resources(self):
        result = self.run_script("key-mismatch")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("match", result.stdout + result.stderr)
        self.assertFalse(self.aws_state["created"])
        self.assertFalse(
            any(c["operation"] == "import-key-pair" for c in self.aws_state["calls"])
        )

    def test_registry_checks_accept_single_image_oci_manifests(self):
        from provision_rhelai import (
            BOOTC_IMAGE,
            BOOTC_REPOSITORY,
            BOOTC_DIGEST,
            INFERENCE_IMAGE,
        )

        self.assertEqual(BOOTC_IMAGE, f"{BOOTC_REPOSITORY}@{BOOTC_DIGEST}")
        self.assertEqual(
            INFERENCE_IMAGE,
            "registry.redhat.io/rhaii/vllm-cuda-rhel9@sha256:0d22e2301ce26ef68effda5d3a4a20381ad0ea34d1240e159e245bd44c2e125b",
        )
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_registry_errors_do_not_falsely_claim_access_denied_or_leak_stderr(self):
        result = self.run_script("registry-local-error")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Registry manifest check failed", result.stderr)
        self.assertNotIn("access denied", result.stderr.lower())
        self.assertNotIn(self.secret, result.stdout + result.stderr)
        self.assertFalse(self.aws_state["created"])

    def test_registry_failure_stops_before_aws_resource_creation(self):
        result = self.run_script("registry-denied")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("registry", result.stderr.lower())
        self.assertFalse(self.aws_state["created"])

    def test_world_readable_auth_file_is_rejected(self):
        self.auth.chmod(0o644)
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("600", result.stderr)
        self.assertFalse(self.aws_state["created"])

    def test_helper_only_auth_file_is_rejected_without_exposing_content(self):
        self.auth.write_text(
            json.dumps({"credHelpers": {"registry.redhat.io": self.secret}})
        )
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("podman login", result.stderr)
        self.assertNotIn(self.secret, result.stdout + result.stderr)
        self.assertFalse(self.aws_state["created"])

    def test_expired_aws_token_stops_before_key_import_or_launch(self):
        result = self.run_script("expired-token")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.aws_state["created"])
        self.assertFalse(
            any(c["operation"] == "import-key-pair" for c in self.aws_state["calls"])
        )

    def test_wrong_instance_never_receives_credentials_or_conversion(self):
        result = self.run_script("wrong-instance")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("identity", result.stdout + result.stderr)
        self.assertFalse(
            any(c["action"] in ("convert", "upload") for c in self.remote["calls"])
        )
        self.assertIn("retained", result.stdout.lower() + result.stderr.lower())

    def test_bootstrap_failures_retain_winner_and_print_exact_cleanup_id(self):
        for scenario in (
            "convert-failure",
            "health-failure",
            "wrong-model",
            "wrong-image",
            "empty-completion",
        ):
            with self.subTest(scenario=scenario):
                # Each subcase needs its own run and mock remote state.
                if self.state_dir.exists():
                    import shutil

                    shutil.rmtree(self.state_dir)
                for name in ("remote.json", "state.json"):
                    (self.root / name).unlink(missing_ok=True)
                result = self.run_script(scenario)
                self.assertNotEqual(result.returncode, 0)
                state = self.load("provision/state.json")
                self.assertIsNotNone(state, "Missing durable provisioning state")
                self.assertIsNotNone(state["winner"])
                winner = state["winner"]["stack_id"]
                deleted = {
                    c["stack"]
                    for c in self.aws_state["calls"]
                    if c["operation"] == "delete-stack"
                }
                self.assertNotIn(winner, deleted)
                self.assertIn(f"--stack-name {winner}", result.stdout + result.stderr)
                self.assert_no_secret()

    def test_resume_reuses_owned_stack_without_new_launch_or_conversion(self):
        first = self.run_script()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        before = len(
            [c for c in self.aws_state["calls"] if c["operation"] == "create-stack"]
        )
        conversions = len([c for c in self.remote["calls"] if c["action"] == "convert"])
        configurations = len(
            [c for c in self.remote["calls"] if c["action"] == "configure"]
        )
        result = self.run_script(resume=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(
            before,
            len(
                [c for c in self.aws_state["calls"] if c["operation"] == "create-stack"]
            ),
        )
        self.assertEqual(
            conversions,
            len([c for c in self.remote["calls"] if c["action"] == "convert"]),
        )
        self.assertEqual(
            configurations,
            len([c for c in self.remote["calls"] if c["action"] == "configure"]),
        )
        self.assert_no_secret()

    def test_resume_after_disconnect_before_conversion_starts(self):
        first = self.run_script()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        path = self.state_dir / "state.json"
        state = json.loads(path.read_text())
        state.update(booted=False, phase="converting")
        path.write_text(json.dumps(state))
        remote_path = self.root / "remote.json"
        remote = json.loads(remote_path.read_text())
        remote.update(converted=False, configured=False)
        before = len([c for c in remote["calls"] if c["action"] == "convert"])
        remote_path.write_text(json.dumps(remote))
        result = self.run_script(resume=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(
            len([c for c in self.remote["calls"] if c["action"] == "convert"]),
            before + 1,
        )

    def test_digest_only_bootc_reference_is_accepted_when_digest_matches(self):
        result = self.run_script("digest-only-ref")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_only_recognized_old_key_is_removed_during_reboot(self):
        result = self.run_script("early-host-key")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("NEWKEY", (self.state_dir / "known_hosts.rhelai").read_text())
        self.assertIn("OLDKEY", (self.state_dir / "known_hosts.rhel").read_text())

    def test_old_ssh_channel_is_retired_while_cloud_user_becomes_ready(self):
        result = self.run_script("delayed-cloud-user")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_resume_after_reboot_ignores_rotated_old_channel(self):
        first = self.run_script()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        path = self.state_dir / "state.json"
        state = json.loads(path.read_text())
        state.update(booted=False, phase="converting")
        path.write_text(json.dumps(state))
        result = self.run_script("delayed-cloud-user", resume=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_unexpected_new_host_key_change_is_rejected(self):
        result = self.run_script("unexpected-host-key")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Unexpected SSH host-key change", result.stderr)
        self.assertIn("NEWKEY", (self.state_dir / "known_hosts.rhelai").read_text())

    def test_signal_does_not_wait_for_a_stalled_ssh_client(self):
        import signal
        import time

        args = [
            "bash",
            str(DIRECTORY / "provision_rhelai.sh"),
            "--registry-authfile",
            str(self.auth),
            "--key-file",
            str(self.key),
            "--state-dir",
            str(self.state_dir),
            "--timeout",
            "30",
            "--poll-interval",
            "0.01",
        ]
        process = subprocess.Popen(
            args,
            env={**self.environment, "MOCK_SCENARIO": "ssh-stalls"},
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        try:
            deadline = time.monotonic() + 8
            while time.monotonic() < deadline:
                remote = self.load("remote.json", {"calls": []})
                if any(c["action"] == "verify" for c in remote["calls"]):
                    break
                time.sleep(0.02)
            else:
                self.fail("Provisioner did not reach the stalled SSH request")
            process.send_signal(signal.SIGTERM)
            stdout, stderr = process.communicate(timeout=3)
            self.assertNotEqual(process.returncode, 0)
            self.assertIn("retained", stdout)
            self.assertIn("interrupted", stderr)
        finally:
            if process.poll() is None:
                process.kill()
                process.communicate()

    def test_bootstrap_starts_before_loser_cleanup_finishes(self):
        result = self.run_script("cleanup-overlap")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        gates = list(self.root.glob("cleanup-gate-*"))
        self.assertEqual(len(gates), 3)
        self.assertTrue(all(path.read_text() == "convert" for path in gates))
        self.assertEqual(self.load("provision/state.json")["phase"], "ready")

    def test_cleanup_failure_does_not_abandon_winner_bootstrap(self):
        result = self.run_script("cleanup-overlap-failure")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(self.remote.get("configured"))
        self.assertEqual(self.load("provision/state.json")["phase"], "ready")
        self.assertIn("cleanup", result.stdout.lower() + result.stderr.lower())
        self.assertEqual(len(list(self.root.glob("cleanup-gate-*"))), 3)

    def test_bootstrap_failure_still_joins_all_loser_cleanup(self):
        result = self.run_script("cleanup-overlap-bootstrap-failure")
        self.assertNotEqual(result.returncode, 0)
        state = self.load("provision/state.json")
        self.assertEqual(
            set(state["cleaned"]),
            set(self.aws_state["created"]) - {state["winner"]["stack_id"]},
        )
        self.assertEqual(len(list(self.root.glob("cleanup-gate-*"))), 3)
        self.assertIn("conversion failed", result.stderr.lower())

    def test_no_winner_does_not_attempt_bootstrap(self):
        result = self.run_script("no-offering")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.remote["calls"])
        self.assertFalse(self.aws_state["created"])

    def test_resume_bootstrap_overlaps_outstanding_cleanup(self):
        first = self.run_script()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        path = self.state_dir / "state.json"
        state = json.loads(path.read_text())
        state["cleaned"] = []
        path.write_text(json.dumps(state))
        aws_path = self.root / "state.json"
        aws = json.loads(aws_path.read_text())
        aws["deleted"] = []
        aws_path.write_text(json.dumps(aws))
        remote_path = self.root / "remote.json"
        remote = json.loads(remote_path.read_text())
        remote["calls"] = []
        remote_path.write_text(json.dumps(remote))
        result = self.run_script("resume-cleanup-overlap", resume=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        gates = list(self.root.glob("cleanup-gate-*"))
        self.assertEqual(len(gates), 3)
        self.assertTrue(all(path.read_text() == "verify" for path in gates))
        self.assertFalse(
            any(call["action"] == "convert" for call in self.remote["calls"])
        )

    def test_resume_retries_failed_loser_cleanup(self):
        first = self.run_script("cleanup-failure")
        self.assertNotEqual(first.returncode, 0)
        before = len(
            [c for c in self.aws_state["calls"] if c["operation"] == "delete-stack"]
        )
        result = self.run_script(resume=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(
            len(
                [c for c in self.aws_state["calls"] if c["operation"] == "delete-stack"]
            ),
            before + 3,
        )
        state = self.load("provision/state.json")
        self.assertEqual(
            set(state["cleaned"]),
            set(self.aws_state["created"]) - {state["winner"]["stack_id"]},
        )

    def test_resume_reports_failed_cleanup_without_reconverting_winner(self):
        first = self.run_script("cleanup-failure")
        self.assertNotEqual(first.returncode, 0)
        self.assertTrue(self.remote.get("configured"))
        before = len([c for c in self.remote["calls"] if c["action"] == "convert"])
        result = self.run_script("cleanup-failure", resume=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(
            before, len([c for c in self.remote["calls"] if c["action"] == "convert"])
        )
        self.assertEqual(self.load("provision/state.json")["phase"], "ready")
        self.assertIn("cleanup", result.stdout.lower() + result.stderr.lower())

    def test_simultaneous_resume_cleanup_and_bootstrap_errors_are_visible(self):
        first = self.run_script("cleanup-failure")
        self.assertNotEqual(first.returncode, 0)
        self.assertTrue(self.remote.get("configured"))
        result = self.run_script("resume-both-fail", resume=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("timeout", result.stderr.lower())
        self.assertIn("coordination failed", result.stdout.lower())
        self.assertIn("owned loser", result.stdout.lower())

    def test_resume_recovers_an_owned_winner_missing_from_checkpoint(self):
        first = self.run_script()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        path = self.state_dir / "state.json"
        state = json.loads(path.read_text())
        winner = state["winner"]
        state["winner"] = None
        path.write_text(json.dumps(state))
        result = self.run_script(resume=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.load("provision/state.json")["winner"], winner)

    def test_winner_is_persisted_before_optional_output_fetch(self):
        import threading
        from dataclasses import asdict
        from deploy_cuda_x86_64 import Attempt
        from provision_rhelai import RhelaiDeployment, State

        self.state_dir.mkdir(mode=0o700)
        attempt = Attempt("us-east-1", "us-east-1a", "owned-stack-id")
        state = State(
            self.state_dir / "state.json",
            {"created": [asdict(attempt)], "winner": None},
        )
        state.update()
        deployment = object.__new__(RhelaiDeployment)
        deployment.state = state
        deployment.stop = threading.Event()
        deployment.winner_lock = threading.Lock()
        deployment.winner_ready = threading.Event()
        deployment.winner = None
        deployment.interrupted = False
        deployment.log = lambda message: None

        def outputs(*args):
            self.assertTrue(deployment.winner_ready.is_set())
            self.assertEqual(
                json.loads(state.path.read_text())["winner"], asdict(attempt)
            )
            return "public outputs"

        deployment.aws = outputs
        deployment.select_winner(attempt)

    def test_resume_rejects_untracked_winner(self):
        first = self.run_script()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        path = self.state_dir / "state.json"
        state = json.loads(path.read_text())
        state["created"] = []
        path.write_text(json.dumps(state))
        result = self.run_script(resume=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("owned", result.stderr.lower())

    def test_state_permissions(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.state_dir.stat().st_mode & 0o777, 0o700)
        self.assertEqual((self.state_dir / "state.json").stat().st_mode & 0o777, 0o600)

    def test_bootc_script_matches_confirmed_command_without_interactive_flags(self):
        path = DIRECTORY / "bootstrap_rhelai.sh"
        self.assertTrue(path.exists(), "Missing bootc bootstrap script")
        source = path.read_text()
        for fragment in (
            "--privileged",
            "/dev:/dev",
            "/var/lib/containers:/var/lib/containers",
            "/:/target",
            "--pid=host",
            "label=type:unconfined_t",
            "bootc install to-existing-root",
            "--root-ssh-authorized-keys /target/root/.ssh/authorized_keys",
            "--skip-fetch-check",
            "systemctl reboot",
            "--tensor-parallel-size $TENSOR_PARALLEL_SIZE",
            "--model $MODEL",
            "--max-model-len 4096",
        ):
            self.assertIn(fragment, source)
        self.assertNotIn("podman run -it", source)
        self.assertNotIn("HUGGING_FACE_HUB_TOKEN", source)

    def test_quadlet_expands_selected_model_and_parallelism(self):
        import re

        source = (DIRECTORY / "bootstrap_rhelai.sh").read_text()
        body = re.search(r"zz-rhelai-provision.conf <<EOF\n(.*?)\nEOF", source, re.S)[1]
        result = subprocess.run(
            [
                "bash",
                "-c",
                "MODEL=RedHatAI/gemma-3-12b-it; TENSOR_PARALLEL_SIZE=4; MAX_NUM_SEQS=16; INFERENCE_IMAGE=pinned; cat <<EOF\n"
                + body
                + "\nEOF\n",
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(
            "Exec=--model RedHatAI/gemma-3-12b-it --tensor-parallel-size 4 --max-model-len 4096 --max-num-seqs 16",
            result.stdout,
        )

    def test_bootstrap_rejects_parallelism_exceeding_visible_gpus(self):
        import re

        source = (DIRECTORY / "bootstrap_rhelai.sh").read_text()
        guard = re.search(r"        gpu_count=.*?\n        fi", source, re.S)[0]
        script = (
            'set -e; TENSOR_PARALLEL_SIZE=4; nvidia-smi() { printf "0\\n1\\n"; };\n'
            + guard
        )
        result = subprocess.run(["bash", "-c", script], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exceeds visible GPUs (2)", result.stderr)

    def test_bootstrap_stages_files_into_the_new_ostree_var(self):
        import re

        source = (DIRECTORY / "bootstrap_rhelai.sh").read_text()
        stage = re.search(r"stage_for_reboot\(\) \{\n.*?\n\}", source, re.S)
        self.assertIsNotNone(stage, "Original RHEL /var is not the new bootc /var")
        phase = re.search(r"phase\(\) \{\n.*?\n\}", source, re.S)
        old = self.root / "old-var"
        old.mkdir(mode=0o700)
        (old / "auth.json").write_bytes(self.auth.read_bytes())
        (old / "auth.json").chmod(0o600)
        (old / "agent.sh").write_text(source)
        (old / "agent.sh").chmod(0o700)
        (old / "install.log").write_text("conversion log")
        target = self.root / "bootc-var" / "run"
        script = (
            'set -euo pipefail; umask 077; ROOT="$1"; AUTH_FILE="$ROOT/auth.json";\n'
        )
        script += phase[0] + "\n" + stage[0] + '\nstage_for_reboot "$2"\n'
        result = subprocess.run(
            ["bash", "-c", script, "bash", str(old), str(target)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((target / "auth.json").read_bytes(), self.auth.read_bytes())
        self.assertEqual((target / "auth.json").stat().st_mode & 0o777, 0o600)
        self.assertEqual((target / "agent.sh").read_text(), source)
        self.assertEqual((target / "phase").read_text().strip(), "rebooting")
        self.assertEqual(target.stat().st_mode & 0o777, 0o700)
        self.assertFalse(
            (old / "auth.json").exists(), "Old root must not retain credentials"
        )
        self.assertIn("/ostree/deploy/default/var", source)

    def test_staging_failure_removes_both_secret_copies(self):
        import re

        source = (DIRECTORY / "bootstrap_rhelai.sh").read_text()
        functions = [
            re.search(rf"{name}\(\) \{{\n.*?\n\}}", source, re.S)[0]
            for name in ("phase", "stage_for_reboot", "failed")
        ]
        old = self.root / "failed-stage"
        old.mkdir()
        (old / "auth.json").write_bytes(self.auth.read_bytes())
        target = self.root / "partial-stage"
        shell_options = re.search(r"^set -[^\n]+", source, re.M)[0]
        script = shell_options + '\nROOT="$1"; AUTH_FILE="$ROOT/auth.json";\n'
        script += "\n".join(functions) + "\ntrap failed ERR\n"
        script += 'cp() { command cp "$ROOT/auth.json" "$STAGED_ROOT/auth.json"; return 42; };\n'
        script += 'stage_for_reboot "$2"\n'
        result = subprocess.run(
            ["bash", "-c", script, "bash", str(old), str(target)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 42)
        self.assertFalse((old / "auth.json").exists())
        self.assertFalse((target / "auth.json").exists())
        self.assertEqual((old / "phase").read_text().strip(), "failed")
        self.assertEqual((target / "phase").read_text().strip(), "failed")
        self.assertNotIn(self.secret, result.stdout + result.stderr)

    def test_metadata_failure_records_failure_and_removes_credentials(self):
        directory = self.root / "metadata-failed"
        directory.mkdir()
        agent = directory / "agent.sh"
        agent.write_bytes((DIRECTORY / "bootstrap_rhelai.sh").read_bytes())
        (directory / "auth.json").write_bytes(self.auth.read_bytes())
        python = self.root / "bin" / "python3"
        python.write_text("#!/bin/sh\nexit 9\n")
        python.chmod(0o755)
        result = subprocess.run(
            ["bash", str(agent), "convert", "dummy-os", "dummy-runtime", "i-owned"],
            env=self.environment,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 9)
        self.assertFalse((directory / "auth.json").exists())
        self.assertEqual((directory / "phase").read_text().strip(), "failed")

    def test_cache_directories_are_writable_by_the_image_group(self):
        source = (DIRECTORY / "bootstrap_rhelai.sh").read_text()
        self.assertIn("install -d -o 0 -g 0 -m 0775", source)
        for name in ("cache", "models", "tmp", "config"):
            self.assertIn("/var/lib/rhaii/" + name, source)
        self.assertIn("nvidia-ctk cdi list 2>/dev/null || true", source)

    def test_generated_quadlet_contains_model_gpu_and_port_arguments(self):
        import re

        generator = Path("/usr/libexec/podman/quadlet")
        if not generator.exists():
            self.skipTest("Optional local Quadlet generator not installed")
        directory = self.root / "quadlets"
        directory.mkdir()
        (directory / "rhaiis.container").write_text(
            "[Container]\nContainerName=rhaii\nImage=example.com/placeholder\nSecurityLabelDisable=true\n"
        )
        dropins = directory / "rhaiis.container.d"
        dropins.mkdir()
        source = (DIRECTORY / "bootstrap_rhelai.sh").read_text()
        config = re.search(r"zz-rhelai-provision.conf <<EOF\n(.*?)\nEOF", source, re.S)[
            1
        ]
        (dropins / "zz-rhelai-provision.conf").write_text(
            config.replace("$INFERENCE_IMAGE", "example.com/pinned-runtime:1")
            .replace("$MODEL", "ibm-granite/granite-3.3-2b-instruct")
            .replace("$TENSOR_PARALLEL_SIZE", "1")
            .replace("$MAX_NUM_SEQS", "16")
        )
        result = subprocess.run(
            [str(generator), "-dryrun", "-no-kmsg-log"],
            env={**os.environ, "QUADLET_UNIT_DIRS": str(directory)},
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        for fragment in (
            "--device nvidia.com/gpu=all",
            "--publish 8000:8000",
            "--shm-size 4g",
            "--model ibm-granite/granite-3.3-2b-instruct",
            "--tensor-parallel-size 1",
            "--max-model-len 4096",
            "--max-num-seqs 16",
        ):
            self.assertIn(fragment, result.stdout + result.stderr)
        self.assertIn("multi-user.target", result.stdout + result.stderr)

    def test_provisioning_entrypoints_have_model_agnostic_names(self):
        for name in (
            "provision_rhelai.sh",
            "provision_rhelai.py",
            "bootstrap_rhelai.sh",
            "test_provision_rhelai.py",
        ):
            self.assertTrue(
                (DIRECTORY / name).exists(), f"Missing generic entrypoint: {name}"
            )
        source = (DIRECTORY / "provision_rhelai.py").read_text()
        self.assertIn("class RhelaiDeployment(Deployment):", source)
        self.assertIn('f"{username}-rhelai-stack"', source)

    def test_launch_template_names_allow_coexisting_stacks(self):
        source = (DIRECTORY / "cuda_cloud_formation_x86_64.yaml").read_text()
        self.assertIn(
            'LaunchTemplateName: !Sub "${AWS::StackName}-cuda-common"', source
        )


class DefaultAuthFileTests(unittest.TestCase):
    def test_native_podman_paths_and_override_precedence(self):
        from unittest.mock import patch
        import provision_rhelai as provisioner

        self.assertTrue(
            hasattr(provisioner, "registry_authfile"),
            "Missing Podman auth-file defaults",
        )
        with patch.dict(os.environ, {}, clear=True), patch.object(
            provisioner.sys, "platform", "linux"
        ), patch.object(provisioner.os, "getuid", return_value=1234):
            self.assertEqual(
                provisioner.registry_authfile(), Path("/run/containers/1234/auth.json")
            )
            os.environ["XDG_RUNTIME_DIR"] = "/run/user/1234"
            self.assertEqual(
                provisioner.registry_authfile(),
                Path("/run/user/1234/containers/auth.json"),
            )
            os.environ["REGISTRY_AUTH_FILE"] = "/custom/auth.json"
            self.assertEqual(provisioner.registry_authfile(), Path("/custom/auth.json"))
            self.assertEqual(
                provisioner.registry_authfile(Path("/explicit/auth.json")),
                Path("/explicit/auth.json"),
            )
        with patch.dict(os.environ, {}, clear=True), patch.object(
            provisioner.sys, "platform", "darwin"
        ), patch.object(Path, "home", return_value=Path("/home/example")):
            self.assertEqual(
                provisioner.registry_authfile(),
                Path("/home/example/.config/containers/auth.json"),
            )


if __name__ == "__main__":
    unittest.main()
