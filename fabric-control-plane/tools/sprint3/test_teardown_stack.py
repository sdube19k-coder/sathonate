import unittest
from unittest.mock import Mock

from teardown_stack import ACCOUNT, REGION, RUN, teardown, verify_inventory


def empty_inventory():
    return [{key: []} for key in (
        "clusters", "Volumes", "NetworkInterfaces", "Addresses", "Vpcs",
        "Reservations", "NatGateways", "LoadBalancers",
    )]


class TeardownTests(unittest.TestCase):
    def setUp(self):
        self.who = {
            "Account": ACCOUNT,
            "Arn": f"arn:aws:sts::{ACCOUNT}:assumed-role/AWSReservedSSO_Sprint3-permissions_0123456789abcdef/sathonate-sprint3",
        }
        self.stack_id = f"arn:aws:cloudformation:{REGION}:{ACCOUNT}:stack/{RUN}/abcd-1234"
        self.stack = {"Stacks": [{
            "StackId": self.stack_id,
            "StackStatus": "CREATE_COMPLETE",
            "Tags": [{"Key": "SathonateRun", "Value": RUN}],
        }]}

    def test_missing_stack_is_idempotent(self):
        call = Mock(side_effect=[self.who, None, *empty_inventory()])
        self.assertIn("absent", teardown(call))
        self.assertEqual(call.call_count, 10)

    def test_foreign_or_untagged_stack_is_refused(self):
        self.stack["Stacks"][0]["Tags"] = []
        call = Mock(side_effect=[self.who, self.stack])
        with self.assertRaisesRegex(RuntimeError, "ownership"):
            teardown(call)
        self.assertEqual(call.call_count, 2)

    def test_deletes_by_immutable_id(self):
        call = Mock(side_effect=[self.who, self.stack, {}, None, *empty_inventory()])
        self.assertIn("deleted", teardown(call))
        self.assertEqual(call.call_args_list[2].args, (
            "cloudformation", "delete-stack", "--stack-name", self.stack_id,
        ))

    def test_timeout_never_reports_success(self):
        call = Mock(side_effect=[self.who, self.stack, {}])
        with self.assertRaises(TimeoutError):
            teardown(call, clock=Mock(side_effect=[0, 20]), timeout=10)

    def test_failed_deletion_can_be_resumed_after_repair(self):
        failed = {"Stacks": [{**self.stack["Stacks"][0],
                              "StackStatus": "DELETE_FAILED",
                              "StackStatusReason": "Fixture dependency"}]}
        pending = {"Stacks": [{**self.stack["Stacks"][0],
                               "StackStatus": "DELETE_IN_PROGRESS"}]}
        call = Mock(side_effect=[
            self.who, self.stack, {}, failed,
            self.who, failed, {}, pending, None, *empty_inventory(),
        ])
        with self.assertRaisesRegex(RuntimeError, "Fixture dependency"):
            teardown(call)
        self.assertIn("deleted", teardown(call, pause=Mock()))
        self.assertEqual(
            [entry.args[2:] for entry in call.call_args_list
             if entry.args[:2] == ("cloudformation", "delete-stack")],
            [("--stack-name", self.stack_id)] * 2,
        )

    def test_existing_deletion_is_polled_without_resubmitting(self):
        self.stack["Stacks"][0]["StackStatus"] = "DELETE_IN_PROGRESS"
        call = Mock(side_effect=[self.who, self.stack, None, *empty_inventory()])
        self.assertIn("deleted", teardown(call))
        self.assertFalse(any(entry.args[:2] == ("cloudformation", "delete-stack")
                             for entry in call.call_args_list))

    def test_running_update_is_cancelled_before_deletion(self):
        stack = self.stack["Stacks"][0]
        stack["StackStatus"] = "UPDATE_IN_PROGRESS"
        rolled_back = {"Stacks": [{**stack, "StackStatus": "UPDATE_ROLLBACK_COMPLETE"}]}
        call = Mock(side_effect=[self.who, self.stack, {}, rolled_back, {}, None, *empty_inventory()])
        self.assertIn("deleted", teardown(call, pause=Mock()))
        self.assertEqual(call.call_args_list[2].args[:2], ("cloudformation", "cancel-update-stack"))
        self.assertEqual(call.call_args_list[4].args[:2], ("cloudformation", "delete-stack"))

    def test_orphan_volume_prevents_success(self):
        inventory = empty_inventory()
        inventory[1]["Volumes"] = [{"VolumeId": "vol-fixture"}]
        with self.assertRaisesRegex(RuntimeError, "Volumes"):
            verify_inventory(Mock(side_effect=inventory))

    def test_untagged_load_balancer_in_run_vpc_prevents_success(self):
        inventory = empty_inventory()
        inventory[-1]["LoadBalancers"] = [{"VpcId": "vpc-abcd"}]
        with self.assertRaisesRegex(RuntimeError, "load balancer"):
            verify_inventory(Mock(side_effect=inventory), "vpc-abcd")
