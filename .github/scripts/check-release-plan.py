#!/usr/bin/env python3
"""Reject a release plan that changes anything other than the service image."""
import json
import sys

plan = json.load(sys.stdin)
for resource in plan.get("resource_changes", []):
    change = resource["change"]
    actions = change["actions"]
    if actions in (["no-op"], ["read"]):
        continue
    address = resource["address"]
    if address != "google_cloud_run_v2_service.api[0]":
        sys.exit(f"Release includes unrelated change: {address} {actions}")
    if actions == ["create"]:
        continue
    if actions != ["update"]:
        sys.exit(f"Release would replace or delete Cloud Run: {actions}")
    before, after = change["before"], change["after"]
    # Cloud Console records its client name; Terraform clears it when applying.
    if before.get("client") == "cloud-console" and after.get("client") is None:
        before.pop("client", None)
        after.pop("client", None)
    for value in (before, after):
        value["template"][0]["containers"][0].pop("image", None)
    if before != after:
        sys.exit("Release contains Cloud Run changes beyond the image; apply infrastructure changes first")
print("Release plan contains only permitted changes.")
