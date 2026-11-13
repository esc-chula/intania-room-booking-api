import os
import subprocess
import sys
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("check-database-target.py")


class DatabaseTargetTest(unittest.TestCase):
    def check(self, url, connection="project:asia-southeast1:room-booking-postgres"):
        env = dict(os.environ, PROJECT_ID="project", REGION="asia-southeast1",
                   INSTANCE_CONNECTION_NAME=connection, DATABASE_URL=url)
        return subprocess.run([sys.executable, str(SCRIPT)], env=env, capture_output=True).returncode

    def test_only_expected_proxy_target_is_allowed(self):
        valid = "postgresql://postgres:password@127.0.0.1:5433/room_booking?sslmode=disable"
        self.assertEqual(self.check(valid), 0)
        for url in (valid.replace("127.0.0.1", "example.com"),
                    valid + "&host=other", valid.replace("5433", "5432"),
                    valid.replace("room_booking", "other")):
            with self.subTest(url=url):
                self.assertNotEqual(self.check(url), 0)
        self.assertNotEqual(self.check(valid, "other:asia-southeast1:room-booking-postgres"), 0)
