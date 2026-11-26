"""Forward SNS alarms to the channel-specific webhook stored in Secrets Manager."""
import json
import os
import urllib.request
import boto3


def handler(event, context):
    secret = boto3.client('secretsmanager').get_secret_value(SecretId=os.environ['WEBHOOK_SECRET_ARN'])['SecretString']
    if not secret.startswith('https://hooks.slack.com/services/'):
        raise ValueError('Invalid Slack webhook destination')
    for record in event['Records']:
        sns = record['Sns']
        try:
            alarm = json.loads(sns['Message'])
        except ValueError:
            alarm = {}
        if 'AlarmName' in alarm:
            text = '\n'.join([
                'RN infrastructure: ' + str(alarm['AlarmName']),
                'Status: ' + str(alarm.get('NewStateValue', 'UNKNOWN')),
                str(alarm.get('NewStateReason', '')),
            ])
        else:
            text = str(sns.get('Subject', 'RN infrastructure')) + '\n' + sns['Message']
        payload = json.dumps({'text': text[:12000], 'mrkdwn': False}).encode()
        request = urllib.request.Request(secret, data=payload, headers={'Content-Type': 'application/json'})
        # Suppress network exceptions: urllib errors can contain the secret URL.
        try:
            with urllib.request.urlopen(request, timeout=10) as response:
                success = response.status == 200 and response.read() == b'ok'
        except Exception:
            raise RuntimeError('Slack delivery failed; retry required') from None
        if not success:
            raise RuntimeError('Slack rejected alert; retry required')
    return {'delivered': len(event['Records'])}
