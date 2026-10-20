#!/usr/bin/env python3
"""One bounded console session; require all three complete rounds to pass."""
import base64
import json
import os
import re
import secrets
import shlex
import subprocess
import sys


def main():
    health_path = os.environ.get('SMOKE_HEALTH_PATH', '/ready')
    if health_path not in ('/health', '/ready'):
        print('Unsupported smoke health path', file=sys.stderr)
        return 1
    marker = 'HHC_BIBLE_' + secrets.token_hex(12)
    urls = [f'http://localhost:3500/v1.0/invoke/bible-api/method{health_path}',
            'https://www.alive.org.tw/api/bible/v1/versions',
            'https://www.alive.org.tw/api/bible/v2/versions']
    # Only fixed URLs and a random hex marker enter the remote shell, never user-provided code.
    body = '''set -eu
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
for round in 1 2 3; do
index=0
for url in URLS; do
rc=0
wget -T 5 -S -O "$work/body" "$url" 2>"$work/headers" || rc=$?
code=$(awk '/HTTP\\// {code=$2} END {print code}' "$work/headers")
payload=$(head -c 65537 "$work/body" | base64 | tr -d '\\n')
printf 'MARKER|%s|%s|%s|%s|%s\\n' "$round" "$index" "$rc" "${code:-000}" "$payload"
index=$((index+1))
done
if [ "$round" -lt 3 ]; then sleep 2; fi
done
'''.replace('URLS', ' '.join(shlex.quote(url) for url in urls)).replace('MARKER', marker)
    # ACA console splits words without shell quoting; keep the shell program in one word.
    encoded = base64.b64encode(body.encode()).decode()
    remote = '/bin/sh -c printf${IFS}%s${IFS}' + encoded + '|base64${IFS}-d|/bin/sh'
    command = shlex.join(['az', 'containerapp', 'exec', '-g', os.environ['RESOURCE_GROUP'],
                          '-n', os.environ['API_GATEWAY_APP_NAME'], '--command', remote])
    try:
        result = subprocess.run(['script', '-q', '-e', '-c', command, '/dev/null'],
                                capture_output=True, text=True, timeout=60)
    except subprocess.TimeoutExpired:
        print('Smoke UNKNOWN: console deadline exceeded', file=sys.stderr)
        return 2
    if result.returncode != 0:
        print('Smoke UNKNOWN: console failed (including 403/429); no health conclusion', file=sys.stderr)
        return 2
    # Match only complete nonce-framed records, never an echoed command or a stray HTTP 200.
    records = {}
    pattern = re.compile(re.escape(marker) + r'\|([123])\|([012])\|([0-9]+)\|([0-9]{3})\|([A-Za-z0-9+/=]*)')
    for match in pattern.finditer(result.stdout.replace('\r', '')):
        key = (int(match[1]), int(match[2]))
        if key in records:
            print('Smoke UNKNOWN: duplicate sample', file=sys.stderr)
            return 2
        records[key] = match[3], match[4], match[5]
    if len(records) != 9:
        print('Smoke UNKNOWN: incomplete console samples', file=sys.stderr)
        return 2
    failed = False
    for (round_number, index), (rc, status, encoded) in sorted(records.items()):
        valid = rc == '0' and status == '200'
        try:
            raw = base64.b64decode(encoded, validate=True)
            payload = json.loads(raw) if len(raw) <= 65536 else None
            if index == 0:
                valid = valid and payload == {'status': 'UP'}
            else:
                valid = valid and isinstance(payload, list) and all(
                    isinstance(item, dict) and type(item.get('id')) is int and item['id'] > 0 and
                    isinstance(item.get('code'), str) and isinstance(item.get('name'), str) and
                    type(item.get('updated_at')) is int and (index != 2 or isinstance(item.get('locale'), str))
                    for item in payload)
        except (ValueError, TypeError):
            valid = False
        print(f'Smoke round={round_number} route={index} status={status} valid={valid}')
        failed = failed or not valid
    return 1 if failed else 0


if __name__ == '__main__':
    sys.exit(main())
