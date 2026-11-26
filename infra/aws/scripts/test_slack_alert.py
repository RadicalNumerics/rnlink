import importlib.util
import json
import os
from pathlib import Path
import sys
import types
import unittest
from unittest.mock import MagicMock, patch

boto = types.ModuleType('boto3')
boto.client = MagicMock()
with patch.dict(sys.modules, {'boto3': boto}):
    spec = importlib.util.spec_from_file_location('slack_alert', Path(__file__).with_name('slack_alert.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)

class Alerts(unittest.TestCase):
    def setUp(self):
        boto.client.return_value.get_secret_value.return_value = {'SecretString': 'https://hooks.slack.com/services/test-only'}
        self.event = {'Records': [{'Sns': {'Message': json.dumps({'AlarmName':'rnlink-instance-health', 'NewStateValue':'ALARM', 'NewStateReason':'test'})}}]}

    @patch.dict(os.environ, {'WEBHOOK_SECRET_ARN':'test'})
    def test_delivery(self):
        with patch.object(module.urllib.request, 'urlopen') as send:
            response = send.return_value.__enter__.return_value
            response.status = 200
            response.read.return_value = b'ok'
            self.assertEqual(module.handler(self.event, None), {'delivered':1})
            payload = json.loads(send.call_args.args[0].data)
            self.assertIn('ALARM', payload['text'])
            self.assertFalse(payload['mrkdwn'])

    @patch.dict(os.environ, {'WEBHOOK_SECRET_ARN':'test'})
    def test_network_error_is_redacted(self):
        with patch.object(module.urllib.request, 'urlopen', side_effect=ValueError('secret-url')):
            with self.assertRaisesRegex(RuntimeError, 'Slack delivery failed') as failure:
                module.handler(self.event, None)
            self.assertNotIn('secret-url', str(failure.exception))
            self.assertTrue(failure.exception.__suppress_context__)

if __name__ == '__main__': unittest.main()
