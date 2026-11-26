import importlib.util
import json
from pathlib import Path
import sqlite3
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("backup", Path(__file__).with_name("backup.py"))
backup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup)

class BackupTests(unittest.TestCase):
    def test_live_wal_database_and_identity_restore(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            data = root / "data"
            identity = data / ".config/tsnet-golink/tailscaled.state"
            identity.parent.mkdir(parents=True)
            identity.write_text(json.dumps({"synthetic-state": "test"}))
            (identity.parent / "tailscaled.log1.txt").write_text("must not be backed up")
            with sqlite3.connect(data / "golink.db") as connection:
                connection.execute("PRAGMA journal_mode=WAL")
                connection.execute("CREATE TABLE Links (Short TEXT, Description TEXT)")
                connection.execute("INSERT INTO Links VALUES ('gpus', 'GPU capacity')")
                connection.commit()
                archive = root / "backup.tar.gz"
                backup.create_archive(data, archive)
                restored = root / "restored"
                with tarfile.open(archive) as contents:
                    self.assertEqual(set(contents.getnames()), {"golink.db", ".config/tsnet-golink/tailscaled.state"})
                    contents.extractall(restored, filter="data")
                with sqlite3.connect(restored / "golink.db") as db:
                    self.assertEqual(db.execute("SELECT * FROM Links").fetchall(), [("gpus", "GPU capacity")])
                    self.assertEqual(db.execute("PRAGMA integrity_check").fetchone(), ("ok",))
                self.assertEqual(json.loads((restored / ".config/tsnet-golink/tailscaled.state").read_text()), {"synthetic-state": "test"})

    def test_bad_upload_checksum_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "backup.tar.gz"
            archive.write_bytes(b"synthetic")
            with patch.object(backup.subprocess, "check_output", return_value=b'{"ChecksumSHA256":"wrong"}'):
                with self.assertRaises(RuntimeError):
                    backup.upload(archive, {"region":"us-west-2", "bucket":"test"})

if __name__ == "__main__":
    unittest.main()
