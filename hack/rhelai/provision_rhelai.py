#!/usr/bin/env python3
"""Provision plain RHEL -> RHEL AI bootc -> a verified inference API.

Only a newly owned capacity-search winner, or that run's explicitly resumed
winner, receives bootstrap commands. AWS and SSH use subprocesses; no boto3.
"""

from __future__ import annotations

import argparse
import base64
from dataclasses import asdict
import ipaddress
import json
import math
import os
from pathlib import Path
import re
import shlex
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time
import uuid

from deploy_cuda_x86_64 import Attempt, Deployment, DeploymentError, DIRECTORY, REGIONS

BOOTC_REPOSITORY = "registry.redhat.io/rhelai3/bootc-cuda-rhel9"
BOOTC_DIGEST = "sha256:e0fbb896811ab54d183e23612030072b946674824bad697bf2b3f68362651662"
# Use digest-only references: containers/image rejects combined tag+digest.
BOOTC_IMAGE = f"{BOOTC_REPOSITORY}@{BOOTC_DIGEST}"
INFERENCE_IMAGE = (
    "registry.redhat.io/rhaii/vllm-cuda-rhel9@"
    "sha256:0d22e2301ce26ef68effda5d3a4a20381ad0ea34d1240e159e245bd44c2e125b"
)
MODEL = "ibm-granite/granite-3.3-2b-instruct"
DEFAULT_MAX_NUM_SEQS = 16
LEGACY_MAX_NUM_SEQS = 256
INSTANCE_TYPE_PATTERN = r"(g4dn|g5|g6|g6e|p4d|p4de|p5|p5e|p5en)\.[a-z0-9]+"
MODEL_PATTERN = r"[A-Za-z0-9][A-Za-z0-9_.-]*(/[A-Za-z0-9][A-Za-z0-9_.-]*)?"


def deployment_settings(data):
    """Validate selection, with original defaults for older resume states."""
    settings = {
        "instance_type": data.get("instance_type", "g6.2xlarge"),
        "model": data.get("model", MODEL),
        "tensor_parallel_size": data.get("tensor_parallel_size", 1),
        "max_num_seqs": data.get("max_num_seqs", LEGACY_MAX_NUM_SEQS),
    }
    if not isinstance(settings["instance_type"], str) or not re.fullmatch(
        INSTANCE_TYPE_PATTERN, settings["instance_type"]
    ):
        raise DeploymentError(
            "instance-type must select a supported x86_64 NVIDIA GPU family"
        )
    if not isinstance(settings["model"], str) or not re.fullmatch(
        MODEL_PATTERN, settings["model"]
    ):
        raise DeploymentError(
            "model must be a safe Hugging Face model ID (e.g. RedHatAI/gemma-3-12b-it)"
        )
    size = settings["tensor_parallel_size"]
    if type(size) is not int or not 1 <= size <= 64:
        raise DeploymentError("tensor-parallel-size must be an integer from 1 to 64")
    sequences = settings["max_num_seqs"]
    if type(sequences) is not int or not 1 <= sequences <= 4096:
        raise DeploymentError("max-num-seqs must be an integer from 1 to 4096")
    return settings


# Never print credentials or put them in commands, environment, or saved state.
INSPECT = r"""# RHELAI_INSPECT
import json, pathlib, shutil, subprocess, urllib.request
base = 'http://169.254.169.254/latest/'
request = urllib.request.Request(base + 'api/token', method='PUT',
    headers={'X-aws-ec2-metadata-token-ttl-seconds': '60'})
with urllib.request.urlopen(request, timeout=5) as response:
    token = response.read().decode()
request = urllib.request.Request(base + 'meta-data/instance-id',
    headers={'X-aws-ec2-metadata-token': token})
with urllib.request.urlopen(request, timeout=5) as response:
    instance_id = response.read().decode().strip()
root = pathlib.Path(ROOT)
bootc = None
if shutil.which('bootc'):
    result = subprocess.run(['bootc', 'status', '--json'], capture_output=True,
        text=True, timeout=20)
    if result.returncode == 0:
        bootc = json.loads(result.stdout)
unit = subprocess.run(['systemctl', 'is-active', 'rhelai-configure-' + RUN_ID],
    capture_output=True, text=True, timeout=10)
convert = subprocess.run(['systemctl', 'is-active', 'rhelai-convert-' + RUN_ID],
    capture_output=True, text=True, timeout=10)
print(json.dumps({'instance_id': instance_id,
    'boot_id': pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip(),
    'phase': (root / 'phase').read_text().strip() if (root / 'phase').exists() else '',
    'configure_active': unit.stdout.strip() in ('active', 'activating'),
    'convert_active': convert.stdout.strip() in ('active', 'activating'),
    'bootc': bootc}))
"""

VERIFY = r"""# RHELAI_VERIFY
import json, urllib.request
base = 'http://127.0.0.1:8000'
with urllib.request.urlopen(base + '/health', timeout=10) as response:
    if response.status != 200:
        raise SystemExit('Health check failed')
with urllib.request.urlopen(base + '/v1/models', timeout=10) as response:
    models = json.load(response)
request = urllib.request.Request(base + '/v1/completions',
    data=json.dumps({'model': MODEL, 'prompt': 'What is the capital of France?',
        'max_tokens': 50, 'temperature': 0}).encode(),
    headers={'Content-Type': 'application/json'})
with urllib.request.urlopen(request, timeout=90) as response:
    completion = json.load(response)
print(json.dumps({'models': models, 'completion': completion}))
"""


class SSHError(DeploymentError):
    """SSH transport or remote process failure, possibly transient during boot."""


def private_file(path: Path):
    if not path.is_file() or path.stat().st_mode & 0o077:
        raise DeploymentError(f"Private file required: {path}; use chmod 600")


def registry_authfile(explicit=None) -> Path:
    """Match Podman's primary login file, without reading credential helpers."""
    if explicit is not None:
        return explicit
    override = os.environ.get("REGISTRY_AUTH_FILE")
    if override:
        return Path(override)
    if sys.platform != "linux":
        return Path.home() / ".config/containers/auth.json"
    runtime = os.environ.get("XDG_RUNTIME_DIR")
    if runtime:
        return Path(runtime) / "containers/auth.json"
    return Path(f"/run/containers/{os.getuid()}/auth.json")


def load_auth(path: Path) -> bytes:
    if not path.is_file():
        raise DeploymentError(
            f"Registry auth file not found: {path}; run podman login registry.redhat.io "
            "as this user, or pass --registry-authfile PATH"
        )
    private_file(path)
    data = path.read_bytes()
    try:
        entries = json.loads(data).get("auths", {})
        usable = False
        for registry, entry in entries.items():
            if registry == "registry.redhat.io" or registry.startswith(
                "registry.redhat.io/"
            ):
                if entry.get("identitytoken"):
                    usable = True
                elif entry.get("auth"):
                    decoded = base64.b64decode(entry["auth"], validate=True)
                    username, password = decoded.split(b":", 1)
                    usable = usable or bool(username and password)
        if usable:
            return data
    except (ValueError, TypeError, AttributeError):
        pass
    raise DeploymentError(
        "Registry auth file must contain explicit registry.redhat.io credentials; "
        "create it with podman login --authfile PATH registry.redhat.io "
        "(credential-helper references cannot be transferred)"
    )


def check_registry(path: Path):
    for image in (BOOTC_IMAGE, INFERENCE_IMAGE):
        try:
            result = subprocess.run(
                [
                    "skopeo",
                    "inspect",
                    "--raw",
                    "--authfile",
                    str(path),
                    "docker://" + image,
                ],
                capture_output=True,
                timeout=120,
                start_new_session=True,
            )
        except (subprocess.SubprocessError, OSError) as error:
            raise DeploymentError(
                "Registry metadata inspection failed; check Skopeo/network availability"
            ) from error
        if result.returncode:
            # Do not relay arbitrary authentication-server responses to logs.
            raise DeploymentError(
                f"Registry manifest check failed for {image} (exit {result.returncode}); "
                "this may be an image, registry, network or local-tool error. "
                "Inspect it with skopeo inspect --raw --authfile PATH docker://IMAGE; "
                "authentication is not necessarily the cause"
            )


def prepare_key(path: Path) -> str:
    """Create missing key files atomically without overwriting either half."""
    if not path.exists():
        public = Path(str(path) + ".pub")
        if public.exists():
            raise DeploymentError(
                f"Orphan public key exists: {public}; refusing overwrite"
            )
        path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(
            prefix=".rhelai-key-", dir=path.parent
        ) as temporary:
            generated = Path(temporary) / "key"
            subprocess.run(
                [
                    "ssh-keygen",
                    "-q",
                    "-t",
                    "rsa",
                    "-b",
                    "3072",
                    "-m",
                    "PEM",
                    "-N",
                    "",
                    "-f",
                    str(generated),
                ],
                check=True,
                capture_output=True,
                timeout=120,
            )
            os.link(generated, path)  # O_EXCL semantics: a concurrent key wins safely.
            try:
                os.link(Path(str(generated) + ".pub"), public)
            except OSError:
                if path.stat().st_ino == generated.stat().st_ino:
                    path.unlink()
                raise
    private_file(path)
    result = subprocess.run(
        ["ssh-keygen", "-y", "-P", "", "-f", str(path)],
        capture_output=True,
        text=True,
        timeout=20,
    )
    if result.returncode:
        raise DeploymentError(
            "SSH key must be valid and unencrypted for unattended use"
        )
    return result.stdout.strip()


class State:
    """Private, atomic run records; serialize writes from regional workers."""

    def __init__(self, path: Path, data: dict):
        self.path, self.data = path, data
        self.data.setdefault("cleaned", [])
        self.lock = threading.Lock()

    def update(self, **values):
        with self.lock:
            self.data.update(values)
            self._save()

    def created(self, attempt: Attempt):
        with self.lock:
            self.data["created"].append(asdict(attempt))
            self._save()

    def cleaned(self, attempt: Attempt):
        with self.lock:
            if attempt.stack_id not in self.data["cleaned"]:
                self.data["cleaned"].append(attempt.stack_id)
            self._save()

    def _save(self):
        temporary = None
        try:
            with tempfile.NamedTemporaryFile(
                mode="w", dir=self.path.parent, delete=False
            ) as file:
                temporary = Path(file.name)
                os.fchmod(file.fileno(), 0o600)
                json.dump(self.data, file, indent=2)
                file.write("\n")
                file.flush()
                os.fsync(file.fileno())
            os.replace(temporary, self.path)
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)


class RhelaiDeployment(Deployment):
    """Existing capacity search with durable ownership and local public keys."""

    def __init__(self, state: State, public_key: str):
        # The existing coordinator configures output for nonblocking cleanup.
        self.output_modes = {fd: os.get_blocking(fd) for fd in (1, 2)}
        super().__init__(instance_type=state.data.get("instance_type", "g6.2xlarge"))
        self.state = state
        self.winner_ready = threading.Event()
        self.public_key = public_key
        self.stack_name = state.data["stack_name"]

    def on_created(self, attempt: Attempt):
        self.state.created(attempt)

    def on_cleaned(self, attempt: Attempt):
        self.state.cleaned(attempt)

    def on_winner_selected(self, attempt: Attempt):
        self.state.update(winner=asdict(attempt), phase="ec2-ready")
        # Notify only after the owned election is durably recorded. Bootstrap
        # must not wait for optional output fetching or losing-worker cleanup.
        self.winner_ready.set()

    def restore_output(self):
        for fd, blocking in self.output_modes.items():
            os.set_blocking(fd, blocking)


class Provisioner:
    def __init__(self, args, state: State, deployment: RhelaiDeployment, auth: bytes):
        self.args, self.state, self.deployment, self.auth = (
            args,
            state,
            deployment,
            auth,
        )
        self.model = state.data.get("model", MODEL)
        self.tensor_parallel_size = state.data.get("tensor_parallel_size", 1)
        self.max_num_seqs = state.data.get("max_num_seqs", LEGACY_MAX_NUM_SEQS)
        self.deadline = time.monotonic() + args.timeout
        self.root = f"/var/lib/rhelai-provision/{state.data['run_id']}"

    def remaining(self, limit=180):
        if self.deployment.interrupted:
            raise DeploymentError(
                "Provisioning interrupted; remote jobs may still finish"
            )
        remaining = self.deadline - time.monotonic()
        if remaining <= 0:
            raise DeploymentError(
                "Provisioning timeout; inspect the retained instance/logs"
            )
        return min(limit, remaining)

    def pause(self):
        time.sleep(min(self.args.poll_interval, self.remaining()))

    def ssh(self, user: str, command: list[str], payload=b"", limit=180) -> str:
        phase = "rhelai" if user == "cloud-user" else "rhel"
        hosts = self.state.path.parent / f"known_hosts.{phase}"
        args = [
            "ssh",
            "-i",
            self.state.data["key_file"],
            "-o",
            "BatchMode=yes",
            "-o",
            "IdentitiesOnly=yes",
            "-o",
            "ConnectTimeout=10",
            "-o",
            "StrictHostKeyChecking=accept-new",
            "-o",
            "UpdateHostKeys=no",
            "-o",
            f"UserKnownHostsFile={hosts}",
            "-o",
            "GlobalKnownHostsFile=/dev/null",
            f"{user}@{self.state.data['public_ip']}",
            shlex.join(command),
        ]
        self.remaining(limit)
        end = min(self.deadline, time.monotonic() + limit)
        try:
            process = subprocess.Popen(
                args,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                start_new_session=True,
            )
        except OSError as error:
            raise SSHError("SSH transport is unavailable") from error
        sent = False
        try:
            while True:
                self.remaining()
                if time.monotonic() >= end:
                    raise SSHError("SSH command timed out")
                try:
                    stdout, stderr_bytes = process.communicate(
                        input=None if sent else payload,
                        timeout=min(0.25, end - time.monotonic()),
                    )
                    break
                except subprocess.TimeoutExpired:
                    sent = True
        except BaseException:
            # Cancelling an SSH client is safe: both bootstrap jobs run under
            # systemd, and the instance/stack identity is already recorded.
            process.terminate()
            try:
                process.communicate(timeout=2)
            except subprocess.TimeoutExpired:
                process.kill()
                process.communicate()
            raise
        stderr = stderr_bytes.decode(errors="replace")
        if process.returncode:
            if "REMOTE HOST IDENTIFICATION HAS CHANGED" in stderr:
                if (
                    phase == "rhel"
                    and self.state.data.get("old_boot_id")
                    and (
                        self.state.data.get("booted")
                        or self.state.data["phase"]
                        in ("converting", "rhelai-ready", "configuring", "ready")
                    )
                ):
                    # The old pinned SSH channel is retired after conversion.
                    # Do not accept its new key, erase its entry, or let it
                    # prevent trying the separate cloud-user channel.
                    raise SSHError("Original RHEL SSH channel retired after conversion")
                # A very early post-reboot probe can cache the *old* RHEL key.
                # Only remove our isolated entry if it is exactly that known key,
                # and only before the new boot has ever been authenticated.
                old_hosts = self.state.path.parent / "known_hosts.rhel"

                def keys(path):
                    return {
                        tuple(line.split()[1:3])
                        for line in path.read_text().splitlines()
                        if len(line.split()) >= 3 and not line.startswith("#")
                    }

                if (
                    phase == "rhelai"
                    and not self.state.data.get("booted")
                    and hosts.exists()
                    and old_hosts.exists()
                    and keys(hosts)
                    and keys(hosts) <= keys(old_hosts)
                ):
                    hosts.unlink()
                    return self.ssh(user, command, payload, limit)
                raise DeploymentError(
                    "Unexpected SSH host-key change; refusing connection"
                )
            raise SSHError("SSH or remote command failed: " + stderr.strip()[-1500:])
        return stdout.decode(errors="replace").strip()

    def inspect(self, user):
        code = f"ROOT={self.root!r}\nRUN_ID={self.state.data['run_id']!r}\n" + INSPECT
        info = json.loads(self.ssh(user, ["sudo", "-n", "python3", "-c", code]))
        if info["instance_id"] != self.state.data["instance_id"]:
            raise DeploymentError("Instance identity mismatch; refusing bootstrap")
        return info

    def upload(self, user, name, payload, mode):
        path = f"{self.root}/{name}"
        code = (
            "# RHELAI_UPLOAD\nimport os, pathlib, sys\n"
            f"root = pathlib.Path({self.root!r})\n"
            "root.mkdir(mode=0o700, parents=True, exist_ok=True)\n"
            f"fd = os.open({path!r}, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, {mode})\n"
            "with os.fdopen(fd, 'wb') as file:\n"
            "    file.write(sys.stdin.buffer.read())\n"
            f"    os.fchmod(file.fileno(), {mode})\n"
        )
        self.ssh(user, ["sudo", "-n", "python3", "-c", code], payload)

    def upload_agent(self, user):
        self.upload(user, "auth.json", self.auth, 0o600)
        self.upload(
            user,
            "agent.sh",
            (DIRECTORY / "bootstrap_rhelai.sh").read_bytes(),
            0o700,
        )

    def job(self, user, mode):
        unit = f"rhelai-{'convert' if mode == 'convert' else 'configure'}-{self.state.data['run_id']}"
        command = [
            "sudo",
            "-n",
            "systemd-run",
            "--unit=" + unit,
            "--property=StandardOutput=append:" + self.root + "/install.log",
            "--property=StandardError=append:" + self.root + "/install.log",
        ]
        if mode == "configure":
            command += ["--wait", "--collect"]
        command += [
            "/usr/bin/bash",
            self.root + "/agent.sh",
            mode,
            BOOTC_IMAGE,
            INFERENCE_IMAGE,
            self.state.data["instance_id"],
            self.model,
            str(self.tensor_parallel_size),
            str(self.max_num_seqs),
        ]
        return self.ssh(user, command, limit=self.args.timeout)

    def initial_boot(self):
        print("Waiting for initial RHEL SSH (ec2-user)...", flush=True)
        while True:
            self.remaining()
            try:
                info = self.inspect("ec2-user")
                self.state.update(old_boot_id=info["boot_id"])
                return info
            except SSHError:
                self.pause()

    def booted_image(self, info):
        booted = (info.get("bootc") or {}).get("status", {}).get("booted")
        if not booted:
            return False
        image = booted["image"]
        if image["imageDigest"] != BOOTC_DIGEST or not image["image"][
            "image"
        ].startswith((BOOTC_REPOSITORY + ":", BOOTC_REPOSITORY + "@")):
            raise DeploymentError(
                "Booted an unexpected RHEL AI image; refusing configuration"
            )
        if info["boot_id"] == self.state.data.get("old_boot_id"):
            raise DeploymentError("Boot ID did not change after conversion")
        self.state.update(booted=True, phase="rhelai-ready")
        return True

    def wait_bootc(self):
        print("Waiting for bootc conversion/reboot and cloud-user SSH...", flush=True)
        while True:
            self.remaining()
            try:
                info = self.inspect("cloud-user")
                if self.booted_image(info):
                    return info
            except SSHError:
                pass
            try:
                old = self.inspect("ec2-user")
                if old["phase"] == "failed":
                    raise DeploymentError(
                        "Bootc conversion failed; inspect remote install.log"
                    )
            except SSHError:
                pass
            self.pause()

    def configure(self, info):
        while info.get("configure_active"):
            self.pause()
            info = self.inspect("cloud-user")
        if info["phase"] == "configured":
            return
        print(
            f"Configuring {self.model} (tensor parallelism {self.tensor_parallel_size}, context 4096, max sequences {self.max_num_seqs})...",
            flush=True,
        )
        self.upload_agent("cloud-user")
        self.state.update(phase="configuring")
        self.job("cloud-user", "configure")

    def verify(self):
        print("Waiting for health, model identity and a completion...", flush=True)
        code = f"MODEL={self.model!r}\n" + VERIFY
        while True:
            self.remaining()
            try:
                data = json.loads(
                    self.ssh("cloud-user", ["sudo", "-n", "python3", "-c", code])
                )
            except SSHError:
                self.pause()
                continue
            models = {item["id"] for item in data["models"]["data"]}
            if models != {self.model}:
                raise DeploymentError(f"Unexpected served model IDs: {sorted(models)}")
            choices = data["completion"].get("choices", [])
            if not choices or not choices[0].get("text", "").strip():
                raise DeploymentError("Inference returned an empty completion")
            text = choices[0]["text"].strip()
            if "paris" not in text.lower():
                raise DeploymentError("France completion did not identify Paris")
            self.state.update(phase="ready")
            print("Verified completion: " + text, flush=True)
            return

    def run(self):
        start_conversion = False
        if not self.state.data.get("old_boot_id"):
            info = self.initial_boot()
            if (info.get("bootc") or {}).get("status", {}).get("booted"):
                raise DeploymentError(
                    "Initial instance is already bootc; refusing reconversion"
                )
            start_conversion = True
        elif not self.state.data.get("booted"):
            # A disconnect may have occurred after recording the old boot ID,
            # but before systemd-run accepted the conversion. Only restart an
            # unstarted job; never repeat a partly completed OS installation.
            try:
                old = self.inspect("ec2-user")
                if not old.get("convert_active"):
                    if old["phase"] == "":
                        if (old.get("bootc") or {}).get("status", {}).get("booted"):
                            raise DeploymentError(
                                "Already bootc; refusing reconversion"
                            )
                        self.state.update(old_boot_id=old["boot_id"])
                        start_conversion = True
                    elif old["phase"] in ("pulling", "converting"):
                        raise DeploymentError(
                            "Conversion job stopped; inspect install.log before recovery"
                        )
            except SSHError:
                pass
        if start_conversion:
            self.upload_agent("ec2-user")
            # Save before starting the autonomous job: disconnects are ambiguous.
            self.state.update(phase="converting")
            self.job("ec2-user", "convert")
        info = self.wait_bootc()
        self.configure(info)
        self.verify()


def resume_attempts(state: State, deployment: RhelaiDeployment):
    """Reconcile owned attempts alongside bootstrap of a validated winner."""
    attempts = [Attempt(**record) for record in state.data["created"]]
    for attempt in attempts:
        if attempt.region not in REGIONS or not attempt.stack_id:
            raise DeploymentError("Invalid owned attempt in resume state")
    winner = state.data.get("winner")
    if winner is not None and winner not in state.data["created"]:
        raise DeploymentError("Resume winner is not a recorded owned attempt")
    if winner is not None:
        # Preserve the existing bootstrap phase; only notify the main thread.
        # winner_details performs live stack/instance checks before SSH.
        deployment.winner_ready.set()

    def absent(error):
        return "ValidationError" in str(error) and "does not exist" in str(error)

    def status(attempt):
        try:
            return deployment.status(attempt)
        except DeploymentError as error:
            if absent(error):
                deployment.on_cleaned(attempt)
                return "DELETE_COMPLETE"
            raise

    failures = {}

    def failed(stage, attempt, error):
        message = f"{stage} {attempt.stack_id} in {attempt.region} failed: {error}"
        failures[attempt.stack_id] = message
        deployment.log(message)

    if winner is None:
        # Inspect every candidate before waiting on pending creations. Poll them
        # round-robin so one slow/broken region cannot hide a ready owned winner.
        pending = [a for a in attempts if a.stack_id not in state.data["cleaned"]]
        for index in range(deployment.create_polls):
            if deployment.interrupted or deployment.errors:
                break
            next_pending = []
            for attempt in pending:
                try:
                    current = status(attempt)
                except (DeploymentError, OSError) as error:
                    failed("Recovery lookup of owned attempt", attempt, error)
                    continue
                if current == "CREATE_COMPLETE" and not deployment.interrupted:
                    deployment.on_winner_selected(attempt)
                    winner = asdict(attempt)
                    break
                if current == "CREATE_IN_PROGRESS":
                    next_pending.append(attempt)
            if winner is not None:
                break
            pending = next_pending
            if not pending:
                break
            if index + 1 < deployment.create_polls:
                deployment.stop.wait(deployment.interval)
        if winner is None:
            for attempt in pending:
                failed(
                    "Recovery of owned attempt",
                    attempt,
                    "creation did not complete before recovery stopped",
                )
    for attempt in attempts:
        if asdict(attempt) == winner or attempt.stack_id in state.data["cleaned"]:
            continue
        try:
            if status(attempt) == "DELETE_COMPLETE":
                deployment.on_cleaned(attempt)
            else:
                deployment.cleanup(attempt)
        except (DeploymentError, OSError) as error:
            if absent(error):
                try:
                    deployment.on_cleaned(attempt)
                except OSError as persistence_error:
                    failed(
                        "Recording cleanup of owned loser", attempt, persistence_error
                    )
            else:
                # API and ledger-write failures must not skip other owned IDs.
                failed("Cleanup of owned loser", attempt, error)
    if failures:
        raise DeploymentError(
            "Resume recovery/cleanup failed: " + "; ".join(failures.values())
        )
    if winner is None:
        raise DeploymentError("No owned winner remains; recorded attempts were cleaned")
    if deployment.errors or deployment.interrupted:
        raise DeploymentError(
            "Resume cleanup interrupted or output unavailable; winner retained"
        )


def winner_details(state: State, deployment: RhelaiDeployment):
    winner = state.data.get("winner")
    if (
        not winner
        or winner not in state.data["created"]
        or winner["region"] not in REGIONS
    ):
        raise DeploymentError(
            "No owned winner in this state file; inspect recorded stacks manually"
        )
    region, stack = winner["region"], winner["stack_id"]
    outputs = json.loads(
        deployment.aws(
            region,
            "cloudformation",
            "describe-stacks",
            "--stack-name",
            stack,
            "--query",
            "Stacks[0].Outputs",
            "--output",
            "json",
        )
    )
    outputs = {item["OutputKey"]: item["OutputValue"] for item in outputs}
    if (
        outputs["StackName"] != state.data["stack_name"]
        or outputs["AWSRegion"] != region
    ):
        raise DeploymentError("Saved winner does not match the expected stack/region")
    instance = deployment.aws(
        region,
        "cloudformation",
        "describe-stack-resource",
        "--stack-name",
        stack,
        "--logical-resource-id",
        "EC2InstanceX8664",
        "--query",
        "StackResourceDetail.PhysicalResourceId",
        "--output",
        "text",
    )
    if instance != outputs["X8664InstanceId"]:
        raise DeploymentError("Instance identity differs from the owned stack resource")
    ip = deployment.aws(
        region,
        "ec2",
        "describe-instances",
        "--instance-ids",
        instance,
        "--query",
        "Reservations[0].Instances[0].PublicIpAddress",
        "--output",
        "text",
    )
    ipaddress.IPv4Address(ip)
    if state.data.get("instance_id") and state.data["instance_id"] != instance:
        raise DeploymentError("Saved instance was replaced; refusing bootstrap")
    state.update(instance_id=instance, public_ip=ip)


def provision_with_cleanup(
    args, state: State, deployment: RhelaiDeployment, auth: bytes
):
    """Bootstrap on election; join owned cleanup before the final result."""
    done = threading.Event()
    outcome = {}

    def coordinate():
        try:
            if args.resume:
                resume_attempts(state, deployment)
                outcome["status"] = 0
            else:
                outcome["status"] = deployment.run()
        except BaseException as error:
            outcome["error"] = error
            # Also report the background failure if bootstrap independently
            # fails: its exception must not hide unresolved cleanup diagnostics.
            deployment.log(f"Capacity/cleanup coordination failed: {error}")
        finally:
            done.set()

    # Main retains signal handling and SSH cancellation. The coordinator owns
    # all regional workers and is always joined: no daemonized AWS cleanup.
    thread = threading.Thread(target=coordinate, name="rhelai-capacity-cleanup")
    thread.start()
    try:
        while not deployment.winner_ready.wait(0.1):
            if deployment.interrupted:
                raise DeploymentError(
                    "Provisioning interrupted; waiting for owned cleanup"
                )
            if done.is_set():
                if "error" in outcome:
                    raise outcome["error"]
                raise DeploymentError(
                    "Capacity search ended without a usable owned winner"
                )
        if deployment.interrupted:
            raise DeploymentError("Provisioning interrupted; winner retained")
        deployment.log(
            "Bootstrapping winner now; losing-attempt cleanup continues in parallel."
        )
        winner_details(state, deployment)
        Provisioner(args, state, deployment, auth).run()
        print_access(state)
        if not done.is_set():
            deployment.log(
                "Winner bootstrap is ready; waiting for remaining cleanup before exit."
            )
    finally:
        # Preserve nonblocking logging until every worker has finished, even
        # after bootstrap errors/signals. Never interrupt an AWS create call.
        thread.join()
        deployment.restore_output()
    if deployment.interrupted:
        raise DeploymentError(
            "Provisioning interrupted; winner retained after owned cleanup"
        )
    if "error" in outcome:
        raise outcome["error"]
    if outcome.get("status") != 0:
        raise DeploymentError(
            "Winner bootstrap completed, but capacity search/loser cleanup did not "
            "finish cleanly; inspect recorded owned stack IDs"
        )


def print_access(state: State):
    data = state.data
    hosts = state.path.parent / "known_hosts.rhelai"
    ssh = shlex.join(
        [
            "ssh",
            "-i",
            data["key_file"],
            "-o",
            f"UserKnownHostsFile={hosts}",
            "-o",
            "StrictHostKeyChecking=yes",
            f"cloud-user@{data['public_ip']}",
        ]
    )
    print(
        f"\nAPI: http://{data['public_ip']}:8000/v1\nModel: {data.get('model', MODEL)}\nSSH: {ssh}"
    )
    print(f"Logs: {ssh} 'sudo journalctl -u rhaiis.service -f'")
    print(
        f"Tunnel: {ssh} -N -L 8080:127.0.0.1:8000\nTunnel API: http://127.0.0.1:8080/v1"
    )


def print_retained(state: State):
    winner = state.data.get("winner")
    if winner:
        print(
            f"\nWinner retained (billable): {winner['region']} {winner['stack_id']}",
            flush=True,
        )
        print(
            "Cleanup: "
            + shlex.join(
                [
                    "aws",
                    "cloudformation",
                    "delete-stack",
                    "--region",
                    winner["region"],
                    "--stack-name",
                    winner["stack_id"],
                ]
            ),
            flush=True,
        )
        print(f"State/resume: {state.path}", flush=True)
        print(
            f"Bootstrap log: /var/lib/rhelai-provision/{state.data['run_id']}/install.log",
            flush=True,
        )

    for attempt in state.data["created"]:
        if attempt == winner or attempt["stack_id"] in state.data.get("cleaned", []):
            continue
        print(
            f"Unresolved owned attempt: {attempt['region']} {attempt['stack_id']}",
            flush=True,
        )
        print(
            "Cleanup: "
            + shlex.join(
                [
                    "aws",
                    "cloudformation",
                    "delete-stack",
                    "--region",
                    attempt["region"],
                    "--stack-name",
                    attempt["stack_id"],
                ]
            ),
            flush=True,
        )


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--username",
        default=os.environ.get("USERNAME", ""),
        help="resource/key prefix (or USERNAME)",
    )
    parser.add_argument(
        "--registry-authfile",
        type=Path,
        help="override REGISTRY_AUTH_FILE or Podman's default auth file",
    )
    parser.add_argument(
        "--key-file",
        type=Path,
        help="existing unencrypted private key, or path for a new key",
    )
    parser.add_argument(
        "--instance-type", help="x86_64 NVIDIA GPU instance (default: g6.2xlarge)"
    )
    parser.add_argument("--model", help=f"Hugging Face model ID (default: {MODEL})")
    parser.add_argument(
        "--tensor-parallel-size",
        type=int,
        help="number of GPUs to shard across (default: 1)",
    )
    parser.add_argument(
        "--max-num-seqs",
        type=int,
        help=f"maximum concurrent sequences (1-4096; default: {DEFAULT_MAX_NUM_SEQS})",
    )
    parser.add_argument("--stack-name", help="default: USERNAME-rhelai-stack")
    parser.add_argument("--state-dir", type=Path, help="new private run directory")
    parser.add_argument(
        "--resume", type=Path, help="state.json from this provisioner's owned run"
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=7200,
        help="bootstrap deadline in seconds, excluding AWS capacity search",
    )
    parser.add_argument("--poll-interval", type=float, default=10)
    args = parser.parse_args()
    for name in ("timeout", "poll_interval"):
        value = getattr(args, name)
        if not math.isfinite(value) or value <= 0:
            parser.error(f"{name} must be positive and finite")
    if args.resume and (
        args.state_dir
        or args.stack_name
        or args.key_file
        or args.instance_type is not None
        or args.model is not None
        or args.tensor_parallel_size is not None
        or args.max_num_seqs is not None
    ):
        parser.error(
            "--resume cannot be combined with --state-dir, --stack-name, --key-file, --instance-type, --model, --tensor-parallel-size or --max-num-seqs"
        )
    if not args.resume:
        try:
            selection = deployment_settings(
                {
                    "instance_type": (
                        args.instance_type
                        if args.instance_type is not None
                        else "g6.2xlarge"
                    ),
                    "model": args.model if args.model is not None else MODEL,
                    "tensor_parallel_size": (
                        args.tensor_parallel_size
                        if args.tensor_parallel_size is not None
                        else 1
                    ),
                    "max_num_seqs": (
                        args.max_num_seqs
                        if args.max_num_seqs is not None
                        else DEFAULT_MAX_NUM_SEQS
                    ),
                }
            )
        except DeploymentError as error:
            parser.error(str(error))
        for name, value in selection.items():
            setattr(args, name, value)
    return args


def main() -> int:
    state = None
    deployment = None
    exit_status = 1
    try:
        args = parse_args()
        for executable in ("aws", "ssh", "ssh-keygen", "podman", "skopeo"):
            if shutil.which(executable) is None:
                raise DeploymentError(f"Required local executable: {executable}")
        auth_path = registry_authfile(args.registry_authfile).expanduser().resolve()
        auth = load_auth(auth_path)
        check_registry(auth_path)
        if args.resume:
            path = args.resume.expanduser().resolve()
            private_file(path)
            if path.parent.stat().st_mode & 0o077:
                raise DeploymentError("Resume directory must have mode 700")
            data = json.loads(path.read_text())
            if data.get("schema") != 1 or not re.fullmatch(
                r"[0-9a-f]{12}", data.get("run_id", "")
            ):
                raise DeploymentError("Unsupported or invalid provisioning state")
            if (
                data.get("bootc_image") != BOOTC_IMAGE
                or data.get("inference_image") != INFERENCE_IMAGE
            ):
                raise DeploymentError("Resume state uses different pinned images")
            data.update(deployment_settings(data))
            state = State(path, data)
            username = data["username"]
            key = Path(data["key_file"])
        else:
            username = args.username
            if not re.fullmatch(r"[A-Za-z][A-Za-z0-9-]{0,79}", username):
                raise DeploymentError(
                    "Set USERNAME/--username: letters, numbers, hyphens; begin with a letter"
                )
            stack_name = args.stack_name or f"{username}-rhelai-stack"
            if not re.fullmatch(r"[A-Za-z][A-Za-z0-9-]{0,109}", stack_name):
                raise DeploymentError(
                    "Stack name must be 1-110 letters/numbers/hyphens, beginning with a letter"
                )
            run_id = uuid.uuid4().hex[:12]
            directory = (
                (args.state_dir or Path.home() / ".local/state/rhelai" / run_id)
                .expanduser()
                .resolve()
            )
            if directory.exists():
                raise DeploymentError(
                    "State directory already exists; use --resume STATEFILE"
                )
            directory.mkdir(mode=0o700, parents=True)
            candidates = [
                Path.home() / ".ssh" / f"{username}-keys.pem",
                Path.home() / "rhelai" / f"{username}-keys.pem",
            ]
            key = (
                (
                    args.key_file
                    or next((p for p in candidates if p.exists()), candidates[0])
                )
                .expanduser()
                .resolve()
            )
            state = State(
                directory / "state.json",
                {
                    "schema": 1,
                    "run_id": run_id,
                    "username": username,
                    "key_file": str(key),
                    "stack_name": stack_name,
                    "instance_type": args.instance_type,
                    "model": args.model,
                    "tensor_parallel_size": args.tensor_parallel_size,
                    "max_num_seqs": args.max_num_seqs,
                    "created": [],
                    "winner": None,
                    "phase": "preflight",
                    "bootc_image": BOOTC_IMAGE,
                    "inference_image": INFERENCE_IMAGE,
                },
            )
            state.update()
        public = prepare_key(key)
        os.environ["USERNAME"] = username
        deployment = RhelaiDeployment(state, public)
        signal.signal(signal.SIGINT, deployment.interrupt)
        signal.signal(signal.SIGTERM, deployment.interrupt)
        account = deployment.aws(
            deployment.regions[0],
            "sts",
            "get-caller-identity",
            "--query",
            "Account",
            "--output",
            "text",
        )
        if state.data.get("account") and state.data["account"] != account:
            raise DeploymentError("AWS account differs from the saved provisioning run")
        state.update(account=account)
        print(f"Run state: {state.path}", flush=True)
        provision_with_cleanup(args, state, deployment, auth)
        if deployment.interrupted:
            raise DeploymentError("Provisioning interrupted; winner retained")
        exit_status = 0
    except (
        DeploymentError,
        OSError,
        ValueError,
        KeyError,
        TypeError,
        subprocess.SubprocessError,
        KeyboardInterrupt,
    ) as error:
        if deployment is not None:
            deployment.restore_output()
        try:
            print(f"Provisioning stopped: {error}", file=sys.stderr, flush=True)
        except OSError:
            pass
        exit_status = 1
    finally:
        if state is not None:
            try:
                print_retained(state)
            except OSError:
                pass
    # A signal during final reporting must still change the process exit code.
    return 1 if deployment is not None and deployment.interrupted else exit_status


if __name__ == "__main__":
    raise SystemExit(main())
