import tempfile
import unittest
from contextlib import nullcontext
from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest.mock import Mock, patch

from cleanup_run import (ACCOUNT, REGION, RUN, cleanup, cleanup_storage,
                         remove_detached_run_volumes, wait_for_cleanup)


class CleanupTests(unittest.TestCase):
    def test_completion_signal_interrupts_deadline_wait(self):
        start = datetime(2026, 11, 15, tzinfo=timezone.utc)
        signal = Mock()
        signal.exists.side_effect = [False, True]
        pause = Mock()
        wait_for_cleanup(start + timedelta(minutes=30), signal,
                         now=lambda: start, pause=pause)
        pause.assert_called_once_with(1)

    def test_deadline_triggers_without_signal(self):
        deadline = datetime(2026, 11, 15, tzinfo=timezone.utc)
        signal = Mock()
        signal.exists.return_value = False
        pause = Mock()
        wait_for_cleanup(deadline, signal, now=lambda: deadline, pause=pause)
        pause.assert_not_called()

    def test_detached_fallback_removes_exact_tagged_volume(self):
        volume = {"VolumeId": "vol-ab", "State": "available", "Attachments": [],
                  "Tags": [{"Key": "SathonateRun", "Value": RUN}]}
        call = Mock(side_effect=[{"Volumes": [volume]}, {}, {"Volumes": []}])
        remove_detached_run_volumes(call)
        self.assertEqual(call.call_args_list[1].args, ("ec2", "delete-volume", "--volume-id", "vol-ab"))

    def test_attached_volume_is_never_deleted(self):
        call = Mock(return_value={"Volumes": [{"VolumeId": "vol-ab", "State": "in-use"}]})
        with self.assertRaisesRegex(RuntimeError, "attached or unverified"):
            remove_detached_run_volumes(call)
        call.assert_called_once()

    def test_storage_timeout_refuses_to_remove_csi(self):
        kube = Mock(return_value={})
        call = Mock(return_value={"Volumes": [{"VolumeId": "vol-ab"}]})
        with self.assertRaises(TimeoutError):
            cleanup_storage(call, kube, "tenant", "network",
                            clock=Mock(side_effect=[0, 0, 601]), pause=Mock())

    def test_storage_deletion_uses_namespace_uid_and_waits_for_volume(self):
        metadata = {"uid": "fixture-uid", "labels": {
            "fabric.sathonate.io/tenant": "tenant",
            "fabric.sathonate.io/network": "network",
            "fabric.sathonate.io/project": "token-bound-project",
            "fabric.sathonate.io/profile": "smp-secure-local-v1",
        }}
        kube = Mock(side_effect=[{"metadata": metadata}, {"items": []}, {}, {}, {}])
        call = Mock(side_effect=[{"Volumes": [{"VolumeId": "vol-ab"}]}, {"Volumes": []}])
        pause = Mock()
        cleanup_storage(call, kube, "tenant", "network", pause=pause)
        self.assertEqual(kube.call_args_list[2].kwargs["body"]["preconditions"], {"uid": "fixture-uid"})
        pause.assert_called_once()

    def test_foreign_namespace_refuses_deletion(self):
        kube = Mock(return_value={"metadata": {"uid": "foreign", "labels": {}}})
        with self.assertRaisesRegex(RuntimeError, "ownership"):
            cleanup_storage(Mock(), kube, "tenant", "network")
        kube.assert_called_once()

    def test_stack_then_admin_and_issuer_survives_retry(self):
        issuer = "https://oidc.eks.ap-south-1.amazonaws.com/id/abcd"
        who = {"Account": ACCOUNT, "Arn":
               f"arn:aws:sts::{ACCOUNT}:assumed-role/AWSReservedSSO_Sprint3-permissions_0123456789abcdef/sathonate-sprint3"}
        stack = {"Stacks": [{
            "StackId": f"arn:aws:cloudformation:{REGION}:{ACCOUNT}:stack/{RUN}/abcd-1234",
            "StackStatus": "CREATE_COMPLETE",
            "Tags": [{"Key": "SathonateRun", "Value": RUN}],
            "Outputs": [{"OutputKey": "OIDCIssuer", "OutputValue": issuer}],
        }]}
        call = Mock(side_effect=[who, stack, who, None])
        actions = Mock()
        actions.admin.side_effect = [RuntimeError("temporary admin failure"), None]
        make_kube = Mock(return_value=nullcontext(Mock()))
        with tempfile.TemporaryDirectory() as directory:
            journal = Path(directory) / "issuer"
            args = (call, make_kube, actions.stack, actions.admin, "tenant", "network", journal)
            with self.assertRaisesRegex(RuntimeError, "temporary admin failure"):
                cleanup(*args)
            cleanup(*args)
            self.assertEqual(journal.read_text(), issuer)
        self.assertEqual([entry[0] for entry in actions.mock_calls], ["stack", "admin", "stack", "admin"])
        make_kube.assert_not_called()

    def test_failed_storage_blocks_stack_then_retry_runs_in_order(self):
        issuer = "https://oidc.eks.ap-south-1.amazonaws.com/id/abcd"
        who = {"Account": ACCOUNT, "Arn":
               f"arn:aws:sts::{ACCOUNT}:assumed-role/AWSReservedSSO_Sprint3-permissions_0123456789abcdef/sathonate-sprint3"}
        stack = {"Stacks": [{
            "StackId": f"arn:aws:cloudformation:{REGION}:{ACCOUNT}:stack/{RUN}/abcd-1234",
            "StackStatus": "UPDATE_COMPLETE",
            "Tags": [{"Key": "SathonateRun", "Value": RUN}],
            "Outputs": [{"OutputKey": "OIDCIssuer", "OutputValue": issuer}],
            "Parameters": [{"ParameterKey": "EnableWorkloads", "ParameterValue": "true"}],
        }]}
        cluster = {"cluster": {"arn": f"arn:aws:eks:{REGION}:{ACCOUNT}:cluster/{RUN}",
                               "tags": {"SathonateRun": RUN}, "identity": {"oidc": {"issuer": issuer}}}}
        actions = Mock()
        actions.storage.side_effect = [TimeoutError("PVC timeout"), None]
        with tempfile.TemporaryDirectory() as directory, patch("cleanup_run.cleanup_storage", actions.storage):
            journal = Path(directory) / "issuer"
            call = Mock(side_effect=[who, stack, cluster] * 2)
            args = (call, Mock(return_value=nullcontext(Mock())), actions.stack, actions.admin,
                    "tenant", "network", journal)
            with self.assertRaisesRegex(TimeoutError, "PVC timeout"):
                cleanup(*args)
            actions.stack.assert_not_called()
            actions.admin.assert_not_called()
            cleanup(*args)
        self.assertEqual([entry[0] for entry in actions.mock_calls],
                         ["storage", "storage", "stack", "admin"])
