#!/usr/bin/env python3
"""Publish app, disk, and backup freshness checks without a resident agent."""
import datetime
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
import urllib.request


def collect(config):
    healthy = 0
    expiry_metrics = {}
    try:
        with urllib.request.urlopen("http://127.0.0.1:9091/healthz", timeout=5) as response:
            healthy = int(response.status == 200 and response.read(64).strip() == b"ok")
            expiry = response.headers.get("X-RN-Node-Key-Expiry")
            if healthy and isinstance(expiry, str):
                if expiry == "disabled":
                    remaining = 100 * 365 * 86400  # Explicit non-expiring identity.
                else:
                    deadline = datetime.datetime.fromisoformat(expiry.replace("Z", "+00:00"))
                    if deadline.tzinfo is None:
                        raise ValueError("expiry must include timezone")
                    remaining = max(0, deadline.timestamp() - time.time())
                expiry_metrics["NodeKeyExpirySeconds"] = remaining
    except (OSError, ValueError):
        pass
    used = []
    for path in ("/", config["data"]):
        usage = shutil.disk_usage(path)
        used.append(100 * usage.used / usage.total)
    age = 86400.0
    try:
        last = json.loads(Path(config["last_backup"]).read_text())["time"]
        elapsed = time.time() - float(last)
        if elapsed >= -60:
            age = max(0, elapsed)
    except (OSError, ValueError, KeyError, TypeError):
        pass
    return {"ServiceHealthy": healthy, "DiskUsedPercent": max(used), "BackupAgeSeconds": age, "NodeKeyExpiryAvailable": int(bool(expiry_metrics)), **expiry_metrics}


def warn_expiry(config, metrics):
    remaining = metrics.get("NodeKeyExpirySeconds")
    if remaining is None or remaining > 7 * 86400:
        return
    now = datetime.datetime.now(datetime.timezone.utc)
    day = now.date().isoformat()
    marker = Path(config["expiry_warning_marker"])
    try:
        if json.loads(marker.read_text()).get("day") == day:
            return
    except (OSError, ValueError, AttributeError):
        pass
    deadline = (now + datetime.timedelta(seconds=remaining)).strftime("%Y-%m-%d %H:%M UTC")
    message = (f"Warning: RN's Tailscale node key expires in {remaining / 86400:.1f} days "
               f"(approximately {deadline}). Reauthenticate the RN node before it expires. "
               "This is the Tailscale node key, not the HTTPS certificate. "
               "Warnings stop automatically when the current key has more than seven days remaining.")
    subprocess.run(["aws", "sns", "publish", "--region", config["region"],
                    "--topic-arn", config["alert_topic"], "--subject", "RN Tailscale expiry warning",
                    "--message", message], check=True, stdout=subprocess.DEVNULL)
    # Record only acknowledged publication; failed sends retry next minute.
    temporary = marker.with_suffix(".tmp")
    temporary.write_text(json.dumps({"day": day}))
    os.replace(temporary, marker)


def main():
    config = json.loads(Path(sys.argv[1]).read_text())
    collected = collect(config)
    metrics = [
        {"MetricName": name, "Value": value, "Dimensions": [{"Name": "Service", "Value": "rnlink"}],
         "Unit": {"ServiceHealthy": "Count", "DiskUsedPercent": "Percent", "BackupAgeSeconds": "Seconds", "NodeKeyExpirySeconds": "Seconds", "NodeKeyExpiryAvailable": "Count"}[name]}
        for name, value in collected.items()
    ]
    subprocess.run(["aws", "cloudwatch", "put-metric-data", "--region", config["region"],
                    "--namespace", "RNLink", "--metric-data", json.dumps(metrics)], check=True)
    warn_expiry(config, collected)
    print("Published RN readiness, disk, and backup freshness metrics")


if __name__ == "__main__":
    main()
