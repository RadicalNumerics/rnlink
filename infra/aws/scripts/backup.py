#!/usr/bin/env python3
"""Consistent SQLite + atomic tsnet identity backup; no logs or disk image."""
import base64
import datetime
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import tarfile
import tempfile
import time


def create_archive(data, destination):
    data = Path(data)
    with tempfile.TemporaryDirectory() as scratch:
        root = Path(scratch)
        database = root / "golink.db"
        with sqlite3.connect((data / "golink.db").as_uri() + "?mode=ro", uri=True, timeout=30) as source:
            with sqlite3.connect(database) as backup:
                source.backup(backup, pages=128, sleep=0.05)
                result = backup.execute("PRAGMA integrity_check").fetchall()
                if result != [("ok",)]:
                    raise RuntimeError("backup database integrity check failed")
        # tsnet writes this JSON state atomically. Copy one complete state file;
        # certificates and log files are regenerated rather than archived.
        identity_path = Path(".config/tsnet-golink/tailscaled.state")
        identity = (data / identity_path).read_bytes()
        if not isinstance(json.loads(identity), dict):
            raise RuntimeError("invalid Tailscale state")
        target = root / identity_path
        target.parent.mkdir(parents=True)
        target.write_bytes(identity)
        os.chmod(target, 0o600)
        with tarfile.open(destination, "w:gz") as archive:
            archive.add(database, arcname="golink.db")
            archive.add(target, arcname=str(identity_path))


def upload(archive, config):
    raw = Path(archive).read_bytes()
    checksum = base64.b64encode(hashlib.sha256(raw).digest()).decode()
    timestamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H-%M-%SZ")
    key = f"backups/{timestamp}-{hashlib.sha256(raw).hexdigest()[:12]}.tar.gz"
    response = json.loads(subprocess.check_output([
        "aws", "s3api", "put-object", "--region", config["region"],
        "--bucket", config["bucket"], "--key", key, "--body", str(archive),
        "--server-side-encryption", "AES256", "--checksum-algorithm", "SHA256",
        "--checksum-sha256", checksum, "--output", "json",
    ]))
    if response.get("ChecksumSHA256") != checksum:
        raise RuntimeError("S3 did not confirm the archive checksum")
    return key, len(raw)


def main():
    os.umask(0o077)
    config = json.loads(Path(sys.argv[1]).read_text())
    with tempfile.TemporaryDirectory() as scratch:
        archive = Path(scratch) / "backup.tar.gz"
        create_archive(Path(config["data"]), archive)
        key, size = upload(archive, config)
    # Only a confirmed upload advances the success marker used by monitoring.
    marker = Path(config["last_backup"])
    temporary = marker.with_suffix(".tmp")
    temporary.write_text(json.dumps({"time": time.time(), "key": key, "bytes": size}))
    os.replace(temporary, marker)
    print(f"Backup verified: {size} bytes")


if __name__ == "__main__":
    main()
