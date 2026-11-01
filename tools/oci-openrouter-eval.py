"""Launch isolated OpenCode with an OCI-managed OpenRouter key, then evaluate proposals."""
import argparse
import base64
import json
import os
from pathlib import Path
import queue
import subprocess
import sys
import tempfile
import threading
import urllib.request

import oci


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--opencode', required=True)
    parser.add_argument('--evaluate', required=True)
    parser.add_argument('--secret-name', default='openrouter-api-key')
    parser.add_argument('--model', default='deepseek/deepseek-v4.1-flash')
    args = parser.parse_args()
    print('Loading OpenRouter credential from OCI into memory', file=sys.stderr)
    opencode = str(Path(args.opencode).resolve())
    evaluate = str(Path(args.evaluate).resolve())
    req = urllib.request.Request('http://169.254.169.254/opc/v2/instance/', headers={'Authorization': 'Bearer Oracle'})
    with urllib.request.urlopen(req, timeout=10) as response:
        metadata = json.load(response)
    signer = oci.auth.signers.InstancePrincipalsSecurityTokenSigner()
    region = signer.region
    vault = oci.vault.VaultsClient({'region': region}, signer=signer)
    matches = [s for s in oci.pagination.list_call_get_all_results(vault.list_secrets, metadata['compartmentId']).data
               if s.secret_name == args.secret_name and s.lifecycle_state == 'ACTIVE']
    if len(matches) != 1:
        raise RuntimeError('Expected one active matching OCI secret')
    secrets = oci.secrets.SecretsClient({'region': region}, signer=signer)
    bundle = secrets.get_secret_bundle(matches[0].id, stage='CURRENT').data
    key = base64.b64decode(bundle.secret_bundle_content.content, validate=True).decode().strip()
    if not key or any(c in key for c in '\r\n'):
        raise RuntimeError('OCI secret does not contain a usable single-line API key')
    with tempfile.TemporaryDirectory(prefix='serverbot-openrouter-') as tmp:
        root = Path(tmp)
        config = root / 'config' / 'opencode'
        config.mkdir(parents=True)
        schema = {'type': 'object', 'properties': {'action': {'type': 'string', 'enum': ['whitelist_add', 'status', 'refuse']},
                                                'target': {'type': 'string'}},
                  'required': ['action', 'target'], 'additionalProperties': False}
        settings = {'permissions': [{'action': '*', 'resource': '*', 'effect': 'deny'}],
                    'providers': {'openrouter': {'body': {'max_tokens': 256, 'temperature': 0,
                        'reasoning': {'enabled': False}, 'provider': {'require_parameters': True},
                        'response_format': {'type': 'json_schema', 'json_schema': {'name': 'serverbot_proposal', 'strict': True, 'schema': schema}}}}}}
        (config / 'opencode.json').write_text(json.dumps(settings))
        env = {'PATH': os.environ.get('PATH', '/usr/bin:/bin'), 'HOME': tmp, 'USERPROFILE': tmp,
               'XDG_CONFIG_HOME': str(root / 'config'), 'XDG_DATA_HOME': str(root / 'data'),
               'XDG_CACHE_HOME': str(root / 'cache'), 'XDG_STATE_HOME': str(root / 'state'),
               'TMPDIR': tmp, 'OPENROUTER_API_KEY': key}
        process = subprocess.Popen([opencode, 'serve', '--hostname', '127.0.0.1', '--port', '0'],
                                   cwd=tmp, env=env, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
        print('Started isolated OpenCode runtime', file=sys.stderr)
        ready = queue.Queue()

        def capture_startup():
            values = {}
            sent = False
            for line in process.stdout:
                if line.startswith('server listening on '):
                    values['url'] = line.strip().removeprefix('server listening on ')
                if line.startswith('server password '):
                    values['password'] = line.strip().removeprefix('server password ')
                if len(values) == 2 and not sent:
                    ready.put(values.copy())
                    sent = True
            if not sent:
                ready.put({})

        threading.Thread(target=capture_startup, daemon=True).start()
        try:
            service = ready.get(timeout=30)
            if not service:
                raise RuntimeError('Isolated OpenCode failed to start')
            child_env = {'PATH': env['PATH'], 'SERVERBOT_OPENCODE_URL': service['url'],
                         'SERVERBOT_OPENCODE_PASSWORD': service['password'], 'SERVERBOT_EVAL_MODEL': args.model}
            print('Running synthetic planner evaluation', file=sys.stderr)
            result = subprocess.run([evaluate], cwd=tmp, env=child_env, timeout=15 * 60)
            return result.returncode
        finally:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception as error:
        # SDK/process exception messages can include request details; emit no raw exception.
        print(json.dumps({'error_type': type(error).__name__, 'status': getattr(error, 'status', None),
                          'code': getattr(error, 'code', None)}), file=sys.stderr)
        sys.exit(1)
