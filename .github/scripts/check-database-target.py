#!/usr/bin/env python3
"""Fail closed unless database operations target the configured Cloud SQL proxy."""
import os
from urllib.parse import parse_qs, urlsplit


def main():
    project = os.environ.get("PROJECT_ID", "")
    region = os.environ.get("REGION", "")
    expected = f"{project}:{region}:room-booking-postgres"
    if not project or not region or os.environ.get("INSTANCE_CONNECTION_NAME") != expected:
        raise SystemExit("Cloud SQL connection name does not match the configured project and region")
    try:
        url = urlsplit(os.environ.get("DATABASE_URL", ""))
        valid = (url.scheme in ("postgres", "postgresql") and url.hostname == "127.0.0.1"
                 and url.port == 5433 and url.path == "/room_booking"
                 and parse_qs(url.query, keep_blank_values=True) == {"sslmode": ["disable"]})
    except ValueError:
        valid = False
    if not valid:
        raise SystemExit("DATABASE_ADMIN_URL must target room_booking at 127.0.0.1:5433 with sslmode=disable")


if __name__ == "__main__":
    main()
