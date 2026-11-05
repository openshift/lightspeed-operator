"""In-memory orchestration regressions; no AWS, SSH or credentials accessed."""

from dataclasses import asdict
from pathlib import Path
from types import SimpleNamespace
import tempfile
import threading
import unittest
from unittest.mock import patch

from deploy_cuda_x86_64 import Attempt, DeploymentError
from provision_rhelai import State, provision_with_cleanup, resume_attempts


class OrchestrationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.a = Attempt("us-east-1", "us-east-1a", "owned-a")
        self.b = Attempt("us-east-2", "us-east-2a", "owned-b")
        self.c = Attempt("us-west-2", "us-west-2a", "owned-c")
        self.state = State(
            Path(self.temp.name) / "state.json",
            {
                "created": [asdict(self.a), asdict(self.b), asdict(self.c)],
                "winner": asdict(self.a),
                "phase": "ec2-ready",
            },
        )
        self.state.update()
        self.logs = []
        self.cleaned = []
        self.polled = []
        self.statuses = {
            a.stack_id: "CREATE_COMPLETE" for a in (self.a, self.b, self.c)
        }

        def status(attempt):
            value = self.statuses[attempt.stack_id]
            if isinstance(value, Exception):
                raise value
            return value

        def cleanup(attempt):
            self.cleaned.append(attempt.stack_id)
            self.state.cleaned(attempt)

        def elect(attempt):
            self.state.update(winner=asdict(attempt))
            self.deployment.winner_ready.set()

        self.deployment = SimpleNamespace(
            winner_ready=threading.Event(),
            status=status,
            cleanup=cleanup,
            on_cleaned=self.state.cleaned,
            on_winner_selected=elect,
            errors=[],
            interrupted=False,
            create_polls=1,
            interval=0.01,
            stop=threading.Event(),
            log=self.logs.append,
            restore_output=lambda: None,
        )
        self.deployment.poll = (
            lambda a, *args: self.polled.append(a.stack_id) or "ROLLBACK_COMPLETE"
        )

    def test_cleanup_attempts_remaining_losers_after_one_lookup_failure(self):
        self.statuses[self.b.stack_id] = DeploymentError("lookup failure")
        with self.assertRaises(DeploymentError):
            resume_attempts(self.state, self.deployment)
        self.assertIn(self.c.stack_id, self.cleaned)
        self.assertNotIn(self.a.stack_id, self.cleaned)

    def test_persistence_failure_does_not_skip_other_owned_losers(self):
        original = self.deployment.cleanup

        def cleanup(attempt):
            if attempt == self.b:
                raise OSError("cannot save cleanup ledger")
            original(attempt)

        self.deployment.cleanup = cleanup
        try:
            resume_attempts(self.state, self.deployment)
        except (DeploymentError, OSError):
            pass
        self.assertIn(self.c.stack_id, self.cleaned)
        self.assertNotIn(self.a.stack_id, self.cleaned)

    def test_signal_during_final_reporting_changes_main_exit_status(self):
        import provision_rhelai as provisioner

        self.state.update(
            schema=1,
            run_id="abcdef123456",
            username="test-user",
            key_file="/not-read.pem",
            bootc_image=provisioner.BOOTC_IMAGE,
            inference_image=provisioner.INFERENCE_IMAGE,
        )
        args = SimpleNamespace(
            resume=self.state.path, registry_authfile=Path("/not-read.json")
        )
        self.deployment.regions = ("us-east-1",)
        self.deployment.aws = lambda *args: "123456789012"
        self.deployment.interrupt = lambda *args: None

        def late_signal(state):
            self.deployment.interrupted = True

        with patch.object(provisioner, "parse_args", return_value=args), patch.object(
            provisioner.shutil, "which", return_value="/mock"
        ), patch.object(provisioner, "load_auth", return_value=b""), patch.object(
            provisioner, "check_registry"
        ), patch.object(
            provisioner, "prepare_key", return_value="public-key"
        ), patch.object(
            provisioner, "RhelaiDeployment", return_value=self.deployment
        ), patch.object(
            provisioner.signal, "signal"
        ), patch.object(
            provisioner, "provision_with_cleanup"
        ), patch.object(
            provisioner, "print_retained", side_effect=late_signal
        ):
            self.assertEqual(provisioner.main(), 1)

    def test_recovery_selects_ready_winner_before_waiting_on_pending_attempt(self):
        self.state.update(winner=None)
        self.statuses[self.a.stack_id] = "CREATE_IN_PROGRESS"
        resume_attempts(self.state, self.deployment)
        self.assertEqual(self.state.data["winner"], asdict(self.b))
        self.assertFalse(self.polled, "Recovery blocked on an earlier pending attempt")

    def test_recovery_failure_does_not_skip_later_winner_or_cleanup(self):
        self.state.update(winner=None)
        self.statuses[self.a.stack_id] = DeploymentError("lookup failure")
        with self.assertRaises(DeploymentError):
            resume_attempts(self.state, self.deployment)
        self.assertEqual(self.state.data["winner"], asdict(self.b))
        self.assertIn(self.c.stack_id, self.cleaned)
        self.assertTrue(self.deployment.winner_ready.is_set())

    def test_late_signal_cannot_return_success_after_cleanup_completed(self):
        self.deployment.winner_ready.set()
        self.deployment.run = lambda: 0
        args = SimpleNamespace(resume=False)

        def late_signal(state):
            self.deployment.interrupted = True

        with patch("provision_rhelai.winner_details"), patch(
            "provision_rhelai.Provisioner"
        ) as provisioner, patch(
            "provision_rhelai.print_access", side_effect=late_signal
        ):
            provisioner.return_value.run.return_value = None
            with self.assertRaisesRegex(DeploymentError, "interrupted"):
                provision_with_cleanup(args, self.state, self.deployment, b"")


if __name__ == "__main__":
    unittest.main()
