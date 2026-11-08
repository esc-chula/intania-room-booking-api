import json
import subprocess
import sys
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("check-release-plan.py")


class ReleasePlanTest(unittest.TestCase):
    def run_plan(self, before, after):
        plan = {"resource_changes": [{
            "address": "google_cloud_run_v2_service.api[0]",
            "change": {"actions": ["update"], "before": before, "after": after},
        }]}
        return subprocess.run([sys.executable, str(SCRIPT)], input=json.dumps(plan),
                              text=True, capture_output=True)

    def test_console_client_removal_with_image_update(self):
        before = {"client": "cloud-console", "template": [{"containers": [{"image": "old"}]}]}
        after = {"client": None, "template": [{"containers": [{"image": "new"}]}]}
        self.assertEqual(self.run_plan(before, after).returncode, 0)

    def test_other_changes_still_rejected(self):
        before = {"client": "cloud-console", "template": [{"containers": [{"image": "old", "cpu": 1}]}]}
        after = {"client": None, "template": [{"containers": [{"image": "new", "cpu": 2}]}]}
        self.assertNotEqual(self.run_plan(before, after).returncode, 0)

    def test_unrelated_client_change_rejected(self):
        before = {"client": "other", "template": [{"containers": [{"image": "old"}]}]}
        after = {"client": None, "template": [{"containers": [{"image": "new"}]}]}
        self.assertNotEqual(self.run_plan(before, after).returncode, 0)


if __name__ == "__main__":
    unittest.main()
