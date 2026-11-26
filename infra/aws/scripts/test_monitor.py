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

class Expiry(unittest.TestCase):
    def collect_expiry(self, value):
        with tempfile.TemporaryDirectory() as d, patch.object(monitor.urllib.request, 'urlopen') as request, patch.object(monitor.time, 'time', return_value=1000):
            response = request.return_value.__enter__.return_value
            response.status = 200
            response.read.return_value = b'ok\n'
            response.headers = {'X-RN-Node-Key-Expiry': value} if value is not None else {}
            return monitor.collect({'data': d, 'last_backup': d + '/missing'})

    def test_renewal_moves_warning_deadline(self):
        for remaining in [604800, 604801, 180 * 86400]:
            deadline = monitor.datetime.datetime.fromtimestamp(1000 + remaining, monitor.datetime.timezone.utc).isoformat()
            self.assertEqual(self.collect_expiry(deadline)['NodeKeyExpirySeconds'], remaining)

    def test_disabled_expired_and_unknown(self):
        self.assertGreater(self.collect_expiry('disabled')['NodeKeyExpirySeconds'], 604800)
        self.assertEqual(self.collect_expiry('1970-01-01T00:00:00Z')['NodeKeyExpirySeconds'], 0)
        for value in [None, 'invalid', '2027-01-01T00:00:00']:
            self.assertNotIn('NodeKeyExpirySeconds', self.collect_expiry(value))

class DailyWarnings(unittest.TestCase):
    def test_daily_dedupe_renewal_and_retry(self):
        with tempfile.TemporaryDirectory() as d, patch.object(monitor.subprocess, 'run') as send:
            config = {'region': 'test', 'alert_topic': 'test', 'expiry_warning_marker': d + '/warning.json'}
            marker = Path(config['expiry_warning_marker'])
            monitor.warn_expiry(config, {'NodeKeyExpirySeconds': 604801})
            monitor.warn_expiry(config, {})
            send.assert_not_called()
            monitor.warn_expiry(config, {'NodeKeyExpirySeconds': 604800})
            monitor.warn_expiry(config, {'NodeKeyExpirySeconds': 604700})
            self.assertEqual(send.call_count, 1)
            self.assertNotIn('<@', str(send.call_args))
            marker.write_text(json.dumps({'day':'2000-01-01'}))
            monitor.warn_expiry(config, {'NodeKeyExpirySeconds': 500000})
            self.assertEqual(send.call_count, 2)
            marker.write_text(json.dumps({'day':'2000-01-01'}))
            monitor.warn_expiry(config, {'NodeKeyExpirySeconds': 180 * 86400})
            self.assertEqual(send.call_count, 2)
            send.side_effect = monitor.subprocess.CalledProcessError(1, 'aws')
            with self.assertRaises(monitor.subprocess.CalledProcessError):
                monitor.warn_expiry(config, {'NodeKeyExpirySeconds': 500000})
            self.assertEqual(json.loads(marker.read_text())['day'], '2000-01-01')
