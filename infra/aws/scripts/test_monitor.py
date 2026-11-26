import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch, Mock
import monitor

class Metrics(unittest.TestCase):
    def test_missing_backup_and_failed_health(self):
        with tempfile.TemporaryDirectory() as d, patch.object(monitor.urllib.request, 'urlopen', side_effect=OSError):
            values = monitor.collect({'data': d, 'last_backup': d + '/missing'})
            self.assertEqual(values['ServiceHealthy'], 0)
            self.assertEqual(values['BackupAgeSeconds'], 86400)
            self.assertGreaterEqual(values['DiskUsedPercent'], 0)

    def test_success_and_backup_age(self):
        with tempfile.TemporaryDirectory() as d, patch.object(monitor.urllib.request, 'urlopen') as request, patch.object(monitor.time, 'time', return_value=1000):
            marker = Path(d) / 'last.json'
            marker.write_text(json.dumps({'time': 900}))
            response = request.return_value.__enter__.return_value
            response.status = 200
            response.read.return_value = b'ok\n'
            values = monitor.collect({'data': d, 'last_backup': str(marker)})
            self.assertEqual(values['ServiceHealthy'], 1)
            self.assertEqual(values['BackupAgeSeconds'], 100)

if __name__ == '__main__': unittest.main()
