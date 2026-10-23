#!/usr/bin/env python3
"""Search commercial US regions concurrently; retain only the first success.

Uses the AWS CLI and Python's standard library, not boto3. Missing public EC2
keys are imported before launching workers. Never forcibly terminate workers:
an in-flight create request must return its stack ID before cleanup is possible.
"""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
import argparse
import ipaddress
import math
import os
from pathlib import Path
import re
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import threading
import time
from typing import Optional

REGIONS = ("us-east-1", "us-east-2", "us-west-1", "us-west-2")
DIRECTORY = Path(__file__).resolve().parent
CAPACITY_ERROR = re.compile(
    r"InsufficientInstanceCapacity|not have sufficient .*capacity|insufficient .*capacity",
    re.IGNORECASE,
)


class DeploymentError(RuntimeError):
    """An API error or unsafe state that must not initiate another attempt."""


@dataclass(frozen=True)
class Attempt:
    region: str
    zone: str
    stack_id: str


class Deployment:
    """Coordinate regional workers, cooperative cancellation, and owned cleanup."""

    def __init__(self, instance_type="g6.2xlarge"):
        self.instance_type = instance_type
        self.username = os.environ.get("USERNAME", "")
        if not self.username:
            raise DeploymentError("Set USERNAME to the resource and key pair prefix")
        first_region = os.environ.get("AWS_REGION") or "us-west-2"
        if first_region not in REGIONS:
            raise DeploymentError("AWS_REGION must be a supported commercial US region")
        self.regions = (first_region,) + tuple(r for r in REGIONS if r != first_region)
        try:
            self.interval = float(os.environ.get("STACK_POLL_INTERVAL_SECONDS", "5"))
        except ValueError as error:
            raise DeploymentError(
                "STACK_POLL_INTERVAL_SECONDS must be positive"
            ) from error
        if not math.isfinite(self.interval) or self.interval <= 0:
            raise DeploymentError(
                "STACK_POLL_INTERVAL_SECONDS must be positive and finite"
            )
        self.create_polls = self.positive_integer("STACK_CREATE_MAX_POLLS", 720)
        self.rollback_polls = self.positive_integer("STACK_ROLLBACK_MAX_POLLS", 720)
        if shutil.which("aws") is None:
            raise DeploymentError("AWS CLI is required")
        self.stack_name = f"{self.username}-cuda-x86-64-stack"
        # Optional local key input for end-to-end provisioning; legacy CLI keeps
        # its existing us-west-2 source behavior when this is unset.
        self.public_key: Optional[str] = None
        self.stop = threading.Event()
        self.winner_lock = threading.Lock()
        self.output_lock = threading.Lock()
        self.winner: Optional[Attempt] = None
        self.errors: list[str] = []
        self.interrupted = False
        self.output_failed = False
        self.stdout_fd = sys.stdout.fileno()
        self.stderr_fd = sys.stderr.fileno()
        self.output_modes = {
            fd: os.get_blocking(fd) for fd in (self.stdout_fd, self.stderr_fd)
        }
        # A full, undrained pipe must not hold a worker before resource cleanup.
        # Leave terminals and regular files in their original mode.
        try:
            for fd in self.output_modes:
                if stat.S_ISFIFO(os.fstat(fd).st_mode):
                    os.set_blocking(fd, False)
        except BaseException:
            self.restore_output()
            raise

    def restore_output(self):
        """Restore inherited modes, attempting both descriptors even on failure."""
        try:
            os.set_blocking(self.stdout_fd, self.output_modes[self.stdout_fd])
        finally:
            os.set_blocking(self.stderr_fd, self.output_modes[self.stderr_fd])

    @staticmethod
    def positive_integer(name: str, default: int) -> int:
        value = os.environ.get(name, str(default))
        if not re.fullmatch(r"[1-9][0-9]*", value):
            raise DeploymentError(f"{name} must be a positive integer")
        return int(value)

    @staticmethod
    def write_nonblocking(fd: int, message: str):
        remaining = memoryview(message.encode("utf-8") + b"\n")
        while remaining:
            written = os.write(fd, remaining)
            if written == 0:
                raise OSError("Output made no progress")
            remaining = remaining[written:]

    def emit_locked(self, message: str):
        """Never let closed or backpressured output prevent resource cleanup."""
        if not self.output_failed:
            try:
                self.write_nonblocking(self.stdout_fd, message)
                return
            except (OSError, ValueError):
                self.output_failed = True
                self.errors.append("Standard output is unavailable; search cancelled")
                self.stop.set()
        try:
            self.write_nonblocking(self.stderr_fd, message)
        except (OSError, ValueError):
            pass

    def log(self, message: str):
        with self.output_lock:
            self.emit_locked(message)

    def fail(self, message: str):
        self.stop.set()
        with self.output_lock:
            self.errors.append(message)
            self.emit_locked(message)

    def interrupt(self, signum, _frame):
        # Even Event.set() takes a lock; signals only set this plain flag.
        self.interrupted = True

    def stopping(self) -> bool:
        return self.interrupted or self.stop.is_set()

    @staticmethod
    def aws(region: str, *arguments: str, timeout: int = 120) -> str:
        if arguments[1] == "wait":
            command = [
                "aws",
                arguments[0],
                "wait",
                arguments[2],
                "--region",
                region,
                *arguments[3:],
            ]
        else:
            command = [
                "aws",
                arguments[0],
                arguments[1],
                "--region",
                region,
                *arguments[2:],
            ]
        command += ["--cli-connect-timeout", "10", "--cli-read-timeout", "30"]
        try:
            result = subprocess.run(
                command,
                capture_output=True,
                text=True,
                timeout=timeout,
                # Terminal Ctrl-C must reach the coordinator, not an in-flight create.
                start_new_session=True,
            )
        except (subprocess.TimeoutExpired, OSError) as error:
            raise DeploymentError(
                f"AWS {arguments[1]} in {region} failed: {error}"
            ) from error
        if result.returncode:
            raise DeploymentError(
                f"AWS {arguments[1]} in {region} failed: {result.stderr.strip()}"
            )
        return result.stdout.strip()

    def ingress_cidr(self) -> str:
        cidr = os.environ.get("MY_IP_CIDR", "").strip()
        if not cidr:
            if shutil.which("curl") is None:
                raise DeploymentError(
                    "curl is required for IP detection; set MY_IP_CIDR"
                )
            try:
                result = subprocess.run(
                    [
                        "curl",
                        "--ipv4",
                        "--fail",
                        "--silent",
                        "--show-error",
                        "--connect-timeout",
                        "5",
                        "--max-time",
                        "15",
                        "https://checkip.amazonaws.com",
                    ],
                    capture_output=True,
                    text=True,
                    timeout=20,
                    check=True,
                    start_new_session=True,
                )
                cidr = f"{ipaddress.IPv4Address(result.stdout.strip())}/32"
            except (ValueError, subprocess.SubprocessError, OSError) as error:
                raise DeploymentError(
                    "Failed to detect public IPv4; set MY_IP_CIDR"
                ) from error
        try:
            network = ipaddress.ip_network(cidr, strict=False)
            if network.version != 4 or network.prefixlen == 0 or "/" not in cidr:
                raise ValueError("restricted IPv4 CIDR required")
        except ValueError as error:
            raise DeploymentError(
                "MY_IP_CIDR must be an IPv4 CIDR with prefix 1-32"
            ) from error
        return str(network)

    def preflight(self, temporary: Path) -> dict[str, list[str]]:
        """Complete discovery/key imports before any concurrent stack creation."""
        zones_by_region = {}
        public_key_file = temporary / "public-key"
        for region in self.regions:
            if self.stopping():
                raise DeploymentError("Search cancelled during preflight")
            offerings = self.aws(
                region,
                "ec2",
                "describe-instance-type-offerings",
                "--location-type",
                "availability-zone",
                "--filters",
                f"Name=instance-type,Values={self.instance_type}",
                "--query",
                "sort(InstanceTypeOfferings[].Location)",
                "--output",
                "text",
            )
            zones = offerings.split() if offerings != "None" else []
            if not zones:
                self.log(
                    f"No AZ offers {self.instance_type} in {region}; skipping region."
                )
                continue
            count = self.aws(
                region,
                "ec2",
                "describe-key-pairs",
                "--filters",
                f"Name=key-name,Values={self.username}-keys",
                "--query",
                "length(KeyPairs)",
                "--output",
                "text",
            )
            if count not in ("0", "1"):
                raise DeploymentError(
                    f"Unexpected key-pair lookup result in {region}: {count}"
                )
            if count == "1" and self.public_key is not None:
                existing = self.aws(
                    region,
                    "ec2",
                    "describe-key-pairs",
                    "--key-names",
                    f"{self.username}-keys",
                    "--include-public-key",
                    "--query",
                    "KeyPairs[0].PublicKey",
                    "--output",
                    "text",
                )
                if existing.split()[:2] != self.public_key.split()[:2]:
                    raise DeploymentError(
                        f"Existing {self.username}-keys in {region} does not match "
                        "the local private key; existing keys are never replaced"
                    )
            if count == "0":
                if not public_key_file.exists():
                    public_key = self.public_key
                    if public_key is None:
                        public_key = self.aws(
                            "us-west-2",
                            "ec2",
                            "describe-key-pairs",
                            "--key-names",
                            f"{self.username}-keys",
                            "--include-public-key",
                            "--query",
                            "KeyPairs[0].PublicKey",
                            "--output",
                            "text",
                        )
                    if not public_key or public_key == "None":
                        raise DeploymentError(
                            "Source key in us-west-2 has no public key"
                        )
                    public_key_file.write_text(public_key + "\n")
                if self.stopping():
                    raise DeploymentError("Search cancelled before key import")
                source = "the local private key" if self.public_key else "us-west-2"
                self.log(f"Importing {self.username}-keys into {region} from {source}.")
                self.aws(
                    region,
                    "ec2",
                    "import-key-pair",
                    "--key-name",
                    f"{self.username}-keys",
                    "--public-key-material",
                    f"fileb://{public_key_file}",
                    "--query",
                    "KeyName",
                    "--output",
                    "text",
                )
            zones_by_region[region] = zones
        return zones_by_region

    def status(self, attempt: Attempt) -> str:
        return self.aws(
            attempt.region,
            "cloudformation",
            "describe-stacks",
            "--stack-name",
            attempt.stack_id,
            "--query",
            "Stacks[0].StackStatus",
            "--output",
            "text",
        )

    def poll(self, attempt: Attempt, pending: str, limit: int, *, cleanup=False) -> str:
        for index in range(limit):
            status = self.status(attempt)
            if status != pending:
                return status
            if not cleanup and self.stopping():
                return "CANCELLED"
            if index + 1 < limit:
                if cleanup:
                    time.sleep(self.interval)
                else:
                    self.stop.wait(self.interval)
        raise DeploymentError(f"{pending} polling timed out for {attempt.stack_id}")

    def cleanup(self, attempt: Attempt):
        """Only delete a returned stack ID; never guess ownership from a name."""
        self.log(f"[{attempt.region}] Cleaning up {attempt.stack_id}.")
        status = self.status(attempt)
        if status == "CREATE_IN_PROGRESS":
            status = self.poll(
                attempt, "CREATE_IN_PROGRESS", self.create_polls, cleanup=True
            )
        if status == "ROLLBACK_IN_PROGRESS":
            status = self.poll(
                attempt, "ROLLBACK_IN_PROGRESS", self.rollback_polls, cleanup=True
            )
        if status == "DELETE_COMPLETE":
            self.on_cleaned(attempt)
            return
        if status != "DELETE_IN_PROGRESS":
            self.aws(
                attempt.region,
                "cloudformation",
                "delete-stack",
                "--stack-name",
                attempt.stack_id,
            )
        # Preserve the existing deletion waiter; no worker is terminated mid-call.
        self.aws(
            attempt.region,
            "cloudformation",
            "wait",
            "stack-delete-complete",
            "--stack-name",
            attempt.stack_id,
            timeout=3700,
        )
        self.on_cleaned(attempt)

    def on_cleaned(self, attempt: Attempt):
        """Hook for callers persisting successful owned-stack deletion."""

    def on_winner_selected(self, attempt: Attempt):
        """Hook invoked before optional output fetching for durable election."""

    def capacity_failure(self, attempt: Attempt):
        reason = self.aws(
            attempt.region,
            "cloudformation",
            "describe-stack-events",
            "--stack-name",
            attempt.stack_id,
            "--query",
            "StackEvents[?LogicalResourceId=='EC2InstanceX8664' && ResourceStatus=='CREATE_FAILED'].ResourceStatusReason",
            "--output",
            "text",
        )
        if not CAPACITY_ERROR.search(reason):
            raise DeploymentError(f"Failure was not an EC2 capacity shortage: {reason}")
        other_failures = self.aws(
            attempt.region,
            "cloudformation",
            "describe-stack-events",
            "--stack-name",
            attempt.stack_id,
            "--query",
            "StackEvents[?ResourceStatus=='CREATE_FAILED' && LogicalResourceId!='EC2InstanceX8664' && ResourceType!='AWS::CloudFormation::Stack' && ResourceStatusReason!='Resource creation cancelled'].ResourceStatusReason",
            "--output",
            "text",
        )
        if other_failures and other_failures != "None":
            raise DeploymentError(f"Another resource failed: {other_failures}")

    def select_winner(self, attempt: Attempt):
        # The lock elects exactly one winner, even for simultaneous successes.
        with self.winner_lock:
            if self.stopping():
                return
            self.winner = attempt
            self.stop.set()
            self.on_winner_selected(attempt)
            self.log(
                f"Winner: {attempt.region} ({attempt.zone}), stack {attempt.stack_id}."
            )
            outputs = self.aws(
                attempt.region,
                "cloudformation",
                "describe-stacks",
                "--stack-name",
                attempt.stack_id,
                "--query",
                "Stacks[0].Outputs",
                "--output",
                "table",
            )
            self.log(outputs)
            self.log(
                "Winner retained. Other regional workers are finishing their cleanup."
            )

    def on_created(self, attempt: Attempt):
        """Hook for callers persisting ownership before polling a new stack."""

    def worker(self, region: str, zones: list[str], cidr: str):
        active = None
        cleanup_attempted = False
        try:
            for zone in zones:
                if self.stopping():
                    break
                self.log(f"[{region}] Creating {self.stack_name} in {zone}.")
                try:
                    stack_id = self.aws(
                        region,
                        "cloudformation",
                        "create-stack",
                        "--stack-name",
                        self.stack_name,
                        "--template-body",
                        f"file://{DIRECTORY / 'cuda_cloud_formation_x86_64.yaml'}",
                        "--parameters",
                        f"ParameterKey=username,ParameterValue={self.username}",
                        f"ParameterKey=InstanceType,ParameterValue={self.instance_type}",
                        f"ParameterKey=PublicSubnetAAvailabilityZone,ParameterValue={zone}",
                        f"ParameterKey=AllowedIngressCidr,ParameterValue={cidr}",
                        "--query",
                        "StackId",
                        "--output",
                        "text",
                    )
                except DeploymentError as error:
                    raise DeploymentError(
                        f"{error}. No stack ID received; existing/untracked stacks are not deleted. "
                        f"Inspect {self.stack_name} in {region} before rerunning."
                    ) from error
                if not stack_id or stack_id == "None":
                    raise DeploymentError(
                        f"No stack ID received. Inspect {self.stack_name} in {region}; "
                        "it is not safe to delete an untracked stack automatically."
                    )
                active = Attempt(region, zone, stack_id)
                cleanup_attempted = False
                self.on_created(active)
                status = self.poll(active, "CREATE_IN_PROGRESS", self.create_polls)
                if self.stopping():
                    break
                if status == "CREATE_COMPLETE":
                    self.select_winner(active)
                    break
                if status not in ("ROLLBACK_IN_PROGRESS", "ROLLBACK_COMPLETE"):
                    raise DeploymentError(f"Stack entered unsafe status {status}")
                self.log(f"[{region}] Stack entered {status} in {zone}.")
                self.capacity_failure(active)
                if status == "ROLLBACK_IN_PROGRESS":
                    self.log(
                        f"[{region}] Waiting for rollback before cleanup and retry."
                    )
                    status = self.poll(
                        active, "ROLLBACK_IN_PROGRESS", self.rollback_polls
                    )
                    if self.stopping():
                        break
                    if status != "ROLLBACK_COMPLETE":
                        raise DeploymentError(
                            f"Rollback entered unexpected status {status}"
                        )
                    self.log(f"[{region}] Rollback complete in {zone}.")
                cleanup_attempted = True
                self.cleanup(active)
                active = None
        except Exception as error:
            self.fail(f"[{region}] Search stopped: {error}")
        finally:
            if active is not None and active != self.winner:
                try:
                    if not cleanup_attempted:
                        self.cleanup(active)
                    else:
                        raise DeploymentError("Previous cleanup did not complete")
                except Exception as error:
                    self.fail(
                        f"[{region}] Manual cleanup required for {active.stack_id}: {error}\n"
                        f"aws cloudformation delete-stack --region {region} "
                        f"--stack-name {active.stack_id}"
                    )

    def run(self) -> int:
        cidr = self.ingress_cidr()
        self.log(f"Allowing inbound access from {cidr}")
        with tempfile.TemporaryDirectory(prefix="cuda-deploy-") as temporary:
            zones_by_region = self.preflight(Path(temporary))
        if not zones_by_region:
            self.log(
                "No G6-compatible availability zones were found in the US regions."
            )
            return 1
        with ThreadPoolExecutor(max_workers=len(zones_by_region)) as executor:
            futures = [
                executor.submit(self.worker, region, zones, cidr)
                for region, zones in zones_by_region.items()
            ]
            for future in as_completed(futures):
                # worker handles failures and cleanup; this catches programming errors.
                try:
                    future.result()
                except Exception as error:
                    self.fail(f"Regional worker failed unexpectedly: {error}")
        if self.interrupted:
            self.log(
                "Search interrupted; workers have finished their cleanup attempts."
            )
        if self.winner is not None:
            self.log(f"Retained winner: {self.winner.region}, {self.winner.stack_id}.")
        else:
            self.log(
                "No winner: US regions were exhausted, cancelled, or a search failed."
            )
        return (
            0
            if self.winner is not None and not self.errors and not self.interrupted
            else 1
        )


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Search US regions concurrently for G6 capacity; retain one winner.",
        epilog="Set USERNAME before running. Optional: MY_IP_CIDR, AWS_REGION, "
        "STACK_POLL_INTERVAL_SECONDS, STACK_CREATE_MAX_POLLS, STACK_ROLLBACK_MAX_POLLS.",
    )
    parser.parse_args()
    deployment = None
    try:
        deployment = Deployment()
        signal.signal(signal.SIGINT, deployment.interrupt)
        signal.signal(signal.SIGTERM, deployment.interrupt)
        return deployment.run()
    except (DeploymentError, OSError) as error:
        try:
            print(f"Deployment stopped: {error}", file=sys.stderr, flush=True)
        except (OSError, ValueError):
            pass
        return 1
    finally:
        if deployment is not None:
            deployment.restore_output()


if __name__ == "__main__":
    raise SystemExit(main())
