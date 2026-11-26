#!/usr/bin/env python3
"""Publish app, disk, and backup freshness checks without a resident agent."""
import json
from pathlib import Path
import shutil
import subprocess
import sys
import time
import urllib.request


def collect(config):
    healthy = 0
    try:
        with urllib.request.urlopen("http://127.0.0.1:9091/healthz", timeout=5) as response:
            healthy = int(response.status == 200 and response.read(64).strip() == b"ok")
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
    return {"ServiceHealthy": healthy, "DiskUsedPercent": max(used), "BackupAgeSeconds": age}


def main():
    config = json.loads(Path(sys.argv[1]).read_text())
    metrics = [
        {"MetricName": name, "Value": value, "Dimensions": [{"Name": "Service", "Value": "rnlink"}],
         "Unit": {"ServiceHealthy": "Count", "DiskUsedPercent": "Percent", "BackupAgeSeconds": "Seconds"}[name]}
        for name, value in collect(config).items()
    ]
    subprocess.run(["aws", "cloudwatch", "put-metric-data", "--region", config["region"],
                    "--namespace", "RNLink", "--metric-data", json.dumps(metrics)], check=True)
    print("Published RN readiness, disk, and backup freshness metrics")


if __name__ == "__main__":
    main()
