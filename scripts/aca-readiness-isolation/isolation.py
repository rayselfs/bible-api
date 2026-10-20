#!/usr/bin/env python3
"""Fixed disposable ACA acceptance targets; never a production deploy command."""
import base64
import datetime as dt
import hashlib
import json
import os
import signal
from pathlib import Path
import re
import secrets
import shlex
import subprocess
import sys
import threading
import tempfile
import time
import uuid
import urllib.parse
import urllib.request
import urllib.error

SUBSCRIPTION = '3765a72c-2c30-492c-8b88-7a26522903f8'
PG_IMAGE = 'pgvector/pgvector:0.8.1-pg17@sha256:3e8b3adfd27b5707128f60956f62a793c3c9326ea8cfaf0eab7adccb5d700b21'
OWNER = 'bible-readiness-isolation'
APPS = ('bible-ready-target', 'bible-ready-pg', 'bible-ready-caller')
PHASES = ('baseline', 'prepare_fault', 'fault', 'recover_transition', 'recovered',
          'prepare_rollback_fault', 'rollback_fault', 'rollback_transition', 'rollback', 'stop')


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


class Context:
    def __init__(self, env):
        require(env.get('GITHUB_REF') == 'refs/heads/main', 'Only trusted main may operate isolation')
        require(env.get('GITHUB_REPOSITORY') == 'rayselfs/bible-api', 'Unexpected repository')
        require(env.get('AZURE_SUBSCRIPTION_ID') == SUBSCRIPTION, 'Unexpected subscription')
        for key in ('GITHUB_RUN_ID', 'GITHUB_RUN_ATTEMPT'):
            require(re.fullmatch(r'[1-9][0-9]{0,18}', env.get(key, '')) is not None, 'Invalid run identity')
        require(re.fullmatch(r'[0-9a-f]{40}', env.get('GITHUB_SHA', '')) is not None, 'Invalid source commit')
        self.run = env['GITHUB_RUN_ID'] + '-' + env['GITHUB_RUN_ATTEMPT']
        self.source = env['GITHUB_SHA']
        self.group = 'hhc-bible-ready-' + self.run
        self.group_id = f'/subscriptions/{SUBSCRIPTION}/resourceGroups/{self.group}'
        self.environment_id = self.group_id + '/providers/Microsoft.App/managedEnvironments/bible-ready-env'
        self.infrastructure_group = 'hhc-bible-ready-infra-' + self.run
        self.infrastructure_id = f'/subscriptions/{SUBSCRIPTION}/resourceGroups/{self.infrastructure_group}'
        self.registry_id = f'/subscriptions/{SUBSCRIPTION}/resourceGroups/alive/providers/Microsoft.ContainerRegistry/registries/alive'
        self.pull_role = f'/subscriptions/{SUBSCRIPTION}/providers/Microsoft.Authorization/roleDefinitions/7f951dda-4ed3-4680-a7ca-43fe172d538d'
        self.role_id = self.registry_id + '/providers/Microsoft.Authorization/roleAssignments/' + str(uuid.uuid5(uuid.NAMESPACE_URL, self.environment_id + self.pull_role))
        self.expires = (dt.datetime.now(dt.timezone.utc) + dt.timedelta(hours=2)).isoformat()

    def app_id(self, name):
        require(name in APPS, 'Unexpected app')
        return self.group_id + '/providers/Microsoft.App/containerApps/' + name

    def tags(self):
        return {'hhc-owner': OWNER, 'hhc-run': self.run, 'hhc-source': self.source, 'hhc-expires': self.expires}


def immutable(image, repository):
    require(re.fullmatch(re.escape('alive.azurecr.io/alive/' + repository + '@sha256:') + r'[0-9a-f]{64}', image) is not None,
            'Image must be the expected immutable repository')
    return image


def role_plan(ctx, environment):
    principal = environment.get('identity', {}).get('principalId', '')
    require(re.fullmatch(r'[0-9a-f-]{36}', principal) is not None and str(uuid.UUID(principal)) == principal, 'Missing new environment principal')
    return {'id': ctx.role_id, 'scope': ctx.registry_id, 'principalId': principal,
            'roleDefinitionId': ctx.pull_role, 'run': ctx.run, 'source': ctx.source}


def plan(ctx, bible_image, helper_image):
    immutable(bible_image, 'bible-api')
    immutable(helper_image, 'bible-readiness-fixture')
    def probe(path, port, kind='Readiness'):
        return {'type': kind, 'httpGet': {'path': path, 'port': port},
                'initialDelaySeconds': 10, 'periodSeconds': 10}

    def container(name, image, cpu, memory, env=None, probes=None):
        return {'name': name, 'image': image, 'resources': {'cpu': cpu, 'memory': memory},
                'env': env or [], 'probes': probes or []}

    def app(containers, minimum, maximum, configuration):
        return {'location': 'eastasia', 'tags': ctx.tags(), 'properties': {
            'environmentId': ctx.environment_id, 'workloadProfileName': 'Consumption',
            'configuration': {'activeRevisionsMode': 'Single', **configuration},
            'template': {'revisionSuffix': 'baseline', 'containers': containers,
                         'scale': {'minReplicas': minimum, 'maxReplicas': maximum}, 'volumes': []}}}

    env = [{'name': 'SERVER_PORT', 'value': '9999'}, {'name': 'GIN_MODE', 'value': 'release'},
           {'name': 'POSTGRES_HOST', 'value': 'bible-ready-pg'}, {'name': 'POSTGRES_USER', 'value': 'fixture'},
           {'name': 'POSTGRES_PASS', 'secretRef': 'pg-password'}, {'name': 'POSTGRES_DB', 'value': 'bible_fixture'},
           {'name': 'POSTGRES_SSLMODE', 'value': 'disable'}, {'name': 'DB_MAX_OPEN_CONNS', 'value': '8'},
           {'name': 'DB_MAX_IDLE_CONNS', 'value': '2'}, {'name': 'DB_CONN_MAX_LIFETIME', 'value': '30m'},
           {'name': 'DB_CONN_MAX_IDLE_TIME', 'value': '5m'}, {'name': 'PGAPPNAME', 'value': 'isolation_baseline'}]
    fixture = container('fixture', helper_image, .25, '0.5Gi',
                        [{'name': 'FIXTURE_MODE', 'value': 'idle'}, {'name': 'FIXTURE_RUN', 'value': ctx.run}],
                        [probe('/ready', 8081)])
    registry = {'registries': [{'server': 'alive.azurecr.io', 'identity': 'system-environment'}]}
    target = app([container('bible-api', bible_image, .25, '0.5Gi', env,
                            [probe('/health', 9999, 'Liveness'), probe('/ready', 9999)]), fixture], 1, 3,
                 {**registry, 'dapr': {'enabled': True, 'appId': 'bible-ready-target', 'appPort': 9999,
                                      'appProtocol': 'http', 'logLevel': 'warn'}})
    pg = app([container('postgres', PG_IMAGE, .5, '1Gi',
                        [{'name': 'POSTGRES_USER', 'value': 'fixture'}, {'name': 'POSTGRES_DB', 'value': 'bible_fixture'},
                         {'name': 'POSTGRES_PASSWORD', 'secretRef': 'pg-password'}])], 1, 1,
             {'ingress': {'external': False, 'targetPort': 5432, 'exposedPort': 5432, 'transport': 'tcp'}})
    pg['properties']['template']['containers'][0]['args'] = ['-c', 'max_connections=100']
    caller_fixture = json.loads(json.dumps(fixture))
    caller_fixture['env'][0]['value'] = 'caller'
    caller = app([caller_fixture], 1, 1, {**registry, 'dapr': {'enabled': True, 'appId': 'bible-ready-caller', 'appPort': 8081, 'appProtocol': 'http'}})
    return {'group': {'location': 'eastasia', 'tags': ctx.tags()},
            'environment': {'location': 'eastasia', 'tags': ctx.tags(), 'identity': {'type': 'SystemAssigned'},
                            'properties': {'appLogsConfiguration': {'destination': 'none'},
                                           'infrastructureResourceGroup': ctx.infrastructure_group,
                                           'workloadProfiles': [{'name': 'Consumption', 'workloadProfileType': 'Consumption'}]}},
            'target': target, 'pg': pg, 'caller': caller,
            'role': {'id': ctx.role_id, 'scope': ctx.registry_id, 'roleDefinitionId': ctx.pull_role,
                     'principalId': 'UNRESOLVED_NEW_ENVIRONMENT_IDENTITY'}}


def owned(ctx, cloud, live=False):
    group = cloud.get(ctx.group_id)
    if group is None:
        return None
    tags = group.get('tags', {})
    require(all(tags.get(k) == v for k, v in ctx.tags().items() if k != 'hhc-expires'), 'Foreign group; refuse mutation')
    expiry = dt.datetime.fromisoformat(tags['hhc-expires'].replace('Z', '+00:00'))
    require(expiry.tzinfo is not None, 'Missing expiry timezone')
    if live:
        now = dt.datetime.now(dt.timezone.utc)
        require(now < expiry <= now + dt.timedelta(hours=2, seconds=30), 'Expired or unbounded allocation')
    return group


def grant_reviewed_role(ctx, cloud, review):
    group = owned(ctx, cloud, live=True)
    require(group is not None and group['tags'].get('hhc-role-review') == digest(review), 'Exact external role review missing')
    require(role_plan(ctx, cloud.get(ctx.environment_id)) == review, 'Environment principal changed')
    require(cloud.get(ctx.role_id) is None, 'Role already exists; refuse to replace it')
    return cloud.put(ctx.role_id, {'properties': {'principalId': review['principalId'],
                     'principalType': 'ServicePrincipal', 'roleDefinitionId': ctx.pull_role}})


def cleanup(ctx, cloud):
    if owned(ctx, cloud) is None:
        # A removed RG must not hide a leaked registry role.
        require(cloud.get(ctx.role_id) is None, 'Orphan role requires exact principal review')
        return
    role = cloud.get(ctx.role_id)
    if role:
        environment = cloud.get(ctx.environment_id)
        require(environment is not None, 'Cannot attest role principal without owned environment')
        expected = role_plan(ctx, environment)
        require(role['properties'].get('principalId') == expected['principalId'] and
                role['properties'].get('roleDefinitionId', '').lower() == ctx.pull_role.lower(), 'Foreign registry role; refuse deletion')
        cloud.delete(ctx.role_id)
        require(cloud.get(ctx.role_id) is None, 'Role deletion not confirmed')
    cloud.delete(ctx.group_id)
    require(cloud.get(ctx.group_id) is None, 'Group deletion not confirmed')


def assess(samples, routes, phases, bind_failure):
    if any(s.get('status') != 200 or s.get('valid') is not True for s in samples):
        return 'FAIL'
    if not bind_failure or not samples or not phases or len({s.get('nonce') for s in samples}) != len(samples):
        return 'UNKNOWN'
    if any(b['mono'] - a['mono'] > 3 or b['mono'] < a['mono'] for a, b in zip(samples, samples[1:])):
        return 'UNKNOWN'
    for sample in samples:
        expected = phases.get(sample.get('phase'))
        if expected is None or not routes.get(sample.get('nonce')):
            return 'UNKNOWN'
        allowed = set(expected.get('revisions', [expected.get('revision')]))
        actual = routes[sample['nonce']]
        if len(actual) != 1 or not actual <= allowed:
            return 'FAIL'
    for phase, expected in phases.items():
        records = [s for s in samples if s.get('phase') == phase]
        if len(records) < 2 or records[-1]['mono'] - records[0]['mono'] < expected['seconds']:
            return 'UNKNOWN'
        if any(b['mono'] - a['mono'] > 3 or b['mono'] < a['mono'] for a, b in zip(records, records[1:])):
            return 'UNKNOWN'
        if expected['seconds'] > 0 and {s.get('version') for s in records} != {1, 2}:
            return 'UNKNOWN'
    return 'PASS'


def console_result(code, output, nonce):
    frames = re.findall(r'ISOLATION_' + re.escape(nonce) + r'=([A-Za-z0-9+/=]+)', output)
    if code or len(frames) != 1 or len(frames[0]) > 8_000_000:
        raise RuntimeError('Management console evidence UNKNOWN')
    try:
        return json.loads(base64.b64decode(frames[0], validate=True))
    except (ValueError, json.JSONDecodeError) as exc:
        raise RuntimeError('Management console evidence UNKNOWN') from exc


def fault_window(observations, phase, seconds):
    rows = [o for o in observations if o.get('phase') == phase]
    return (len(rows) >= 2 and rows[-1]['mono'] - rows[0]['mono'] >= seconds and
            all(o.get('held') is True and o.get('health') == 200 and o.get('ready') == 503 and
                o.get('outbound') != 204 for o in rows) and
            all(0 <= b['mono'] - a['mono'] <= 3 for a, b in zip(rows, rows[1:])))


def nonce_routes(logs):
    routes = {}
    for path, revision in logs:
        for line in path.read_text().splitlines():
            try:
                value = json.loads(line)
                message = value.get('Log', value.get('log', ''))
            except (ValueError, AttributeError) as exc:
                raise RuntimeError('Unstructured log evidence UNKNOWN') from exc
            for nonce in re.findall(r'isolation_nonce=(iso-[0-9]+-[0-9]+-[0-9]+)', message):
                routes.setdefault(nonce, set()).add(revision)
    return routes


def bind_fatal(path):
    for line in path.read_text().splitlines():
        try:
            value = json.loads(line)
            message = value.get('Log', value.get('log', '')).lower()
        except (ValueError, AttributeError):
            return False
        if 'fatal' in message and '50002' in message and 'address already in use' in message:
            return True
    return False


def bounded(command, timeout=90, data=None):
    process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, text=True, start_new_session=True)
    try:
        stdout, _ = process.communicate(data, timeout=timeout)
    except subprocess.TimeoutExpired as exc:
        os.killpg(process.pid, signal.SIGTERM)
        process.communicate()
        raise RuntimeError('Management command timed out; result UNKNOWN') from exc
    if process.returncode:
        raise RuntimeError('Management command failed; result UNKNOWN')
    return stdout


class Cloud:
    def __init__(self, ctx):
        self.ctx = ctx

    def allowed(self, resource):
        return resource in {self.ctx.group_id, self.ctx.environment_id, self.ctx.role_id,
                            *(self.ctx.app_id(name) for name in APPS)}

    def request(self, method, resource, body=None):
        require(self.allowed(resource) or (method == 'GET' and resource == self.ctx.infrastructure_id), 'Unreviewed resource target')
        api = '2025-07-01'
        if resource in (self.ctx.app_id(name) for name in APPS):
            api = '2026-01-01'
        if resource == self.ctx.group_id or resource == self.ctx.infrastructure_id:
            api = '2021-04-01'
        elif resource == self.ctx.role_id:
            api = '2022-04-01'
        command = ['az', 'rest', '--method', method, '--url',
                   f'https://management.azure.com{resource}?api-version={api}', '--only-show-errors']
        if body is not None:
            command += ['--body', '@/dev/stdin', '--headers', 'Content-Type=application/json']
        process = subprocess.run(command, input=json.dumps(body) if body is not None else None,
                                 capture_output=True, text=True, timeout=90)
        if method == 'GET' and process.returncode and any(code in process.stderr for code in
                ('ResourceGroupNotFound', 'ResourceNotFound', 'RoleAssignmentNotFound')):
            return None
        if process.returncode:
            # Full ARM responses/CLI errors may contain secret payloads; never emit them.
            raise RuntimeError('Azure management operation failed; result UNKNOWN')
        return json.loads(process.stdout) if process.stdout.strip() else None

    def get(self, resource):
        return self.request('GET', resource)

    def put(self, resource, body):
        return self.request('PUT', resource, body)

    def delete(self, resource):
        self.request('DELETE', resource)
        wait_for(lambda: self.get(resource) is None, 300, 'resource deletion')

    def cli(self, *args):
        return json.loads(bounded(['az', *args, '--only-show-errors', '--output', 'json']))

    def replica(self, app, revision):
        require(app in APPS and revision.startswith(app+'--'), 'Unexpected replica target')
        replicas = self.cli('containerapp', 'replica', 'list', '-g', self.ctx.group, '-n', app, '--revision', revision)
        require(len(replicas) == 1, 'Acceptance requires one actual replica; unexpected scale is UNKNOWN')
        return replicas[0]['name']

    def console(self, app, revision, replica, command):
        require(app in APPS and revision.startswith(app+'--') and replica.startswith(revision+'-'), 'Unexpected console target')
        az = ['az', 'containerapp', 'exec', '-g', self.ctx.group, '-n', app, '--revision', revision,
              '--replica', replica, '--container', 'postgres' if app == 'bible-ready-pg' else 'fixture', '--command', command]
        # One bounded console call. The fixture emits one nonce frame; ANSI/prompt text is not health evidence.
        args = ['script', '-q', '/dev/null', *az] if sys.platform == 'darwin' else [
            'script', '-q', '-e', '-c', shlex.join(az), '/dev/null']
        return bounded(args, 90)

    def control(self, app, revision, replica, action):
        require(action in ('state', 'release') or action in ('phase-'+p for p in PHASES), 'Unexpected control')
        nonce = secrets.token_hex(8)
        output = self.console(app, revision, replica, '/fixture client '+action+' '+nonce)
        return console_result(0, output, nonce)


def wait_for(predicate, seconds, label):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        result = predicate()
        if result:
            return result
        time.sleep(5)
    raise RuntimeError(label+' timed out; result UNKNOWN')


def write_json(folder, name, value):
    (folder/name).write_text(json.dumps(value, indent=2, sort_keys=True)+'\n')


def ready_revision(ctx, cloud, app, suffix, images):
    expected = app+'--'+suffix
    def check():
        current = cloud.get(ctx.app_id(app))['properties']
        if current.get('latestRevisionName') != expected or current.get('latestReadyRevisionName') != expected:
            return False
        revision = cloud.cli('containerapp', 'revision', 'show', '-g', ctx.group, '-n', app, '--revision', expected)['properties']
        require({c['name']:c['image'] for c in revision['template']['containers']} == images, 'Runtime digest mismatch')
        return (revision.get('active') is True and revision.get('healthState') == 'Healthy' and
                revision.get('runningState') in ('Running', 'RunningAtMaxScale'))
    wait_for(check, 600, app+' readiness')
    return expected, cloud.replica(app, expected)


def pg_sql(ctx, cloud, revision, replica, sql):
    nonce = secrets.token_hex(8)
    command = shlex.join(['psql', '--no-psqlrc', '-U', 'fixture', '-d', 'bible_fixture', '-qAt', '-v', 'ON_ERROR_STOP=1', '-c', sql])
    shell = 'set -eu; value="$('+command+')"; printf "ISOLATION_'+nonce+'=%s\\n" "$(printf %s "$value" | base64 | tr -d \'\\n\')"'
    # ACA console does not preserve shell-quoted startup arguments.
    encoded = base64.b64encode(shell.encode()).decode()
    remote = '/bin/sh -c printf${IFS}%s${IFS}' + encoded + '|base64${IFS}-d|/bin/sh'
    output = cloud.console('bible-ready-pg', revision, replica, remote)
    return console_result(0, output, nonce)


def validate_approval(ctx, env):
    review = json.loads(env.get('BIBLE_ACA_ISOLATION_REVIEW', '{}'))
    now = dt.datetime.now(dt.timezone.utc)
    verified = dt.datetime.fromisoformat(review['verifiedAt'].replace('Z', '+00:00'))
    expires = dt.datetime.fromisoformat(review['expiresAt'].replace('Z', '+00:00'))
    require(verified.tzinfo is not None and expires.tzinfo is not None, 'Review timestamps need timezones')
    require(dt.timedelta(0) <= now-verified <= dt.timedelta(minutes=30), 'Stale isolation review')
    require(now < expires <= now+dt.timedelta(hours=2), 'Expired/unbounded isolation review')
    require(review.get('sourceCommit') == ctx.source and review.get('releaseCommit') == env.get('BIBLE_RELEASE_COMMIT') and
            str(review.get('releaseRun')) == env.get('BIBLE_RELEASE_RUN'), 'Artifact review mismatch')
    require(review.get('budgetUsd') == 3 and review.get('maxMinutes') == 120 and review.get('slot'), 'Missing budget/assigned slot review')
    require(review.get('rateSnapshotSha') == env.get('ISOLATION_RATE_SHA') and review.get('rateSnapshotSha'), 'Fresh rate review mismatch')
    not_before = dt.datetime.fromisoformat(review['notBefore'].replace('Z', '+00:00'))
    require(not_before.tzinfo is not None, 'Slot timestamp needs timezone')
    require(now >= not_before, 'Coordinator slot has not started')
    require(not media_frozen(now), 'Media freeze')
    return review


def media_frozen(now):
    taipei = now + dt.timedelta(hours=8)
    return taipei.date() == dt.date(2026, 10, 20) and dt.time(5, 15) <= taipei.time() < dt.time(6)


def release_commit(release):
    require(release.get('path') == '.github/workflows/release.yml' and release.get('head_branch') == 'main' and
            release.get('status') == 'completed' and release.get('conclusion') == 'success' and
            release.get('event') in ('push', 'workflow_dispatch'), 'Successful normal main Production Release required')
    commit = release.get('head_sha', '')
    require(re.fullmatch(r'[0-9a-f]{40}', commit) is not None, 'Invalid release source')
    return commit


def receipt_image(pointer, run, commit):
    require(pointer.get('commit') == commit and pointer.get('releaseUrl') ==
            'https://github.com/rayselfs/bible-api/actions/runs/'+run, 'Current normal Release receipt mismatch')
    return immutable(pointer.get('image', ''), 'bible-api')


def export(**values):
    with open(os.environ['GITHUB_ENV'], 'a') as output:
        for key, value in values.items():
            require('\n' not in value and '\r' not in value, 'Invalid workflow output')
            output.write(key+'='+value+'\n')


def rates(folder):
    query = urllib.parse.urlencode({'$filter': "serviceName eq 'Azure Container Apps' and armRegionName eq 'eastasia' and priceType eq 'Consumption'", 'currencyCode': 'USD'})
    with urllib.request.urlopen('https://prices.azure.com/api/retail/prices?'+query, timeout=30) as response:
        raw = json.load(response)
    require(not raw.get('NextPageLink'), 'Incomplete retail price snapshot')
    names = ('Standard vCPU Active Usage', 'Standard Memory Active Usage', 'Environment Management Hour')
    selected = [{key: row[key] for key in ('meterName', 'retailPrice', 'unitOfMeasure', 'effectiveStartDate', 'currencyCode')}
                for row in raw['Items'] if row.get('isPrimaryMeterRegion') and row['meterName'] in names]
    require(len(selected) == 3, 'Unexpected retail meters')
    selected.sort(key=lambda row: row['meterName'])
    write_json(folder, 'retail-rates.json', selected)
    export(ISOLATION_RATE_SHA=digest(selected))


def resolve_release(ctx, folder):
    run = os.environ['BIBLE_RELEASE_RUN']
    require(re.fullmatch(r'[1-9][0-9]{0,18}', run) is not None, 'Invalid normal release run')
    release = json.loads(bounded(['gh', 'api', 'repos/rayselfs/bible-api/actions/runs/'+run]))
    commit = release_commit(release)
    bounded(['git', 'merge-base', '--is-ancestor', commit, 'origin/main'])
    with tempfile.TemporaryDirectory() as temporary:
        path = Path(temporary)/'current.json'
        bounded(['az', 'storage', 'blob', 'download', '--account-name', 'hhcapidocsprod',
                 '--container-name', 'api-docs-bible-api', '--name', 'current.json', '--auth-mode', 'login',
                 '--file', str(path), '--no-progress', '--only-show-errors', '--output', 'none'])
        pointer = json.loads(path.read_text())
    ref = receipt_image(pointer, run, commit)
    write_json(folder, 'release-receipt.json', pointer)
    write_json(folder, 'bible-provenance.json', {'run': run, 'commit': commit, 'image': ref,
                                               'workflowSource': ctx.source, 'releaseUrl': release['html_url']})
    export(BIBLE_IMAGE_REF=ref, BIBLE_RELEASE_COMMIT=commit)


def build_helper(ctx, folder):
    validate_approval(ctx, os.environ)
    tag = 'alive/bible-readiness-fixture:isolation-'+ctx.run
    require(all(row['name'] != 'isolation-'+ctx.run for row in helper_tags()),
            'Helper tag already exists; refuse overwrite')
    bounded(['az', 'acr', 'build', '--registry', 'alive', '--image', tag,
             '--file', 'scripts/aca-readiness-isolation/Dockerfile', '--build-arg', 'FIXTURE_RUN='+ctx.run,
             '--build-arg', 'FIXTURE_SOURCE='+ctx.source,
             '--no-logs', '--only-show-errors', '.'], 900)
    record = json.loads(bounded(['az', 'acr', 'repository', 'show', '--name', 'alive', '--image', tag,
                                '--output', 'json', '--only-show-errors']))
    image = immutable('alive.azurecr.io/alive/bible-readiness-fixture@'+record['digest'], 'bible-readiness-fixture')
    write_json(folder, 'helper-provenance.json', {'image': image, 'tag': tag, 'run': ctx.run, 'source': ctx.source})
    export(HELPER_IMAGE_REF=image)


def helper_tags():
    repository = 'alive/bible-readiness-fixture'
    repos = json.loads(bounded(['az', 'acr', 'repository', 'list', '--name', 'alive', '--output', 'json', '--only-show-errors']))
    if repository not in repos:
        return []
    return json.loads(bounded(['az', 'acr', 'repository', 'show-tags', '--name', 'alive', '--repository',
                               repository, '--detail', '--output', 'json', '--only-show-errors']))


def helper_labels(sha):
    immutable('alive.azurecr.io/alive/bible-readiness-fixture@'+sha, 'bible-readiness-fixture')
    # Credentials stay in memory; refuse redirects rather than forward registry authorization.
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, req, fp, code, msg, headers, newurl):
            return None
    opener = urllib.request.build_opener(NoRedirect())
    def read(request):
        try:
            with opener.open(request, timeout=30) as response:
                raw = response.read(1_000_001)
        except (urllib.error.URLError, OSError) as exc:
            raise RuntimeError('Helper ownership read unavailable; retain artifact') from exc
        require(len(raw) <= 1_000_000, 'Helper ownership response too large')
        return raw
    login = json.loads(bounded(['az', 'acr', 'login', '--name', 'alive', '--expose-token', '--output', 'json', '--only-show-errors']))
    form = urllib.parse.urlencode({'grant_type': 'refresh_token', 'service': 'alive.azurecr.io',
            'scope': 'repository:alive/bible-readiness-fixture:pull', 'refresh_token': login['accessToken']}).encode()
    token = json.loads(read(urllib.request.Request('https://alive.azurecr.io/oauth2/token', data=form,
                      headers={'Content-Type': 'application/x-www-form-urlencoded'})))['access_token']
    def content(kind, value):
        require(re.fullmatch(r'sha256:[0-9a-f]{64}', value) is not None, 'Invalid helper content digest')
        raw = read(urllib.request.Request('https://alive.azurecr.io/v2/alive/bible-readiness-fixture/'+kind+'/'+value,
                headers={'Authorization': 'Bearer '+token, 'Accept': 'application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json'}))
        require('sha256:'+hashlib.sha256(raw).hexdigest() == value, 'Helper ownership content digest mismatch')
        return json.loads(raw)
    manifest = content('manifests', sha)
    return content('blobs', manifest['config']['digest'])['config'].get('Labels') or {}


def cleanup_helper(ctx, folder):
    cloud = Cloud(ctx)
    require(all(cloud.get(resource) is None for resource in (ctx.group_id,ctx.infrastructure_id,ctx.role_id)),
            'Resources still exist; retain their helper image')
    repository, tag = 'alive/bible-readiness-fixture', 'isolation-'+ctx.run
    def record(value):
        write_json(folder, 'cleanup-helper.json', value)
        return value
    rows = helper_tags()
    own = [row for row in rows if row['name'] == tag]
    if not own:
        return record({'ownedTagAbsent': True, 'unreferencedManifestsNotPurged': True})
    require(len(own) == 1, 'Ambiguous helper image tag')
    sha = own[0]['digest']
    immutable('alive.azurecr.io/'+repository+'@'+sha, 'bible-readiness-fixture')
    labels = helper_labels(sha)
    require(labels.get('hhc.isolation.run') == ctx.run and labels.get('org.opencontainers.image.revision') == ctx.source,
            'Helper immutable ownership differs; retain artifact')
    rows = helper_tags()
    require([row for row in rows if row['name'] == tag] == own, 'Helper tag changed; retain artifact')
    if any(row['digest'] == sha and row['name'] != tag for row in rows):
        return record({'retainedDigest': sha, 'reason': 'other tags exist; no broad purge'})
    bounded(['az', 'acr', 'repository', 'delete', '--name', 'alive', '--image', repository+'@'+sha,
             '--yes', '--only-show-errors'])
    rows = helper_tags()
    require(all(row['name'] != tag for row in rows), 'Helper deletion not confirmed')
    return record({'deletedDigest': sha, 'run': ctx.run, 'source': ctx.source})


def cleanup_attempts(env, folder, helper=False):
    Context(env)
    results = {}
    for attempt in range(1, int(env['GITHUB_RUN_ATTEMPT'])+1):
        ctx = Context(dict(env, GITHUB_RUN_ATTEMPT=str(attempt)))
        evidence = folder/('attempt-'+str(attempt))
        evidence.mkdir(parents=True, exist_ok=True)
        try:
            if helper:
                results[ctx.run] = cleanup_helper(ctx, evidence)
            else:
                cloud = Cloud(ctx)
                cleanup(ctx, cloud)
                wait_for(lambda: cloud.get(ctx.infrastructure_id) is None, 300, 'platform infrastructure group deletion')
                results[ctx.run] = {'verifiedAbsent': True}
        except (ValueError, RuntimeError, KeyError, subprocess.TimeoutExpired):
            # Attempt every deterministic prior allocation even when one needs manual exact cleanup.
            results[ctx.run] = {'result': 'UNKNOWN'}
    complete = all(value.get('result') != 'UNKNOWN' for value in results.values())
    write_json(folder, 'cleanup-helper.json' if helper else 'cleanup.json',
               {'attempts': results, 'verifiedAbsent': complete and not helper, 'result': 'PASS' if complete else 'UNKNOWN'})
    require(complete, 'Cleanup incomplete in at least one run attempt; inspect exact residuals')


def runtime_inventory(ctx, cloud, folder, name, allowed):
    current = cloud.get(ctx.app_id('bible-ready-target'))['properties']
    rows = cloud.cli('containerapp', 'revision', 'list', '-g', ctx.group, '-n', 'bible-ready-target', '--all')
    require(all(type(r['properties'].get('replicas')) is int and 0 <= r['properties']['replicas'] <= 1 for r in rows),
            'Unexpected or unknown replica count; acceptance UNKNOWN')
    live = {r['name'] for r in rows if r['properties'].get('active') is True or
            r['properties'].get('replicas', 0) > 0 or r['properties'].get('runningState') not in ('Stopped', 'Inactive', 'ScaleTo0')}
    require(len(live) <= 3 and live <= allowed, 'Unknown or excessive live revision overlap')
    write_json(folder, 'inventory-'+name+'.json', {'latest': current['latestRevisionName'],
        'latestReady': current['latestReadyRevisionName'], 'revisions': [
         {'name': r['name'], **{key: r['properties'].get(key) for key in ('active', 'replicas', 'healthState', 'runningState')},
          'images': [c['image'] for c in r['properties']['template']['containers']],
          'probes': [c.get('probes', []) for c in r['properties']['template']['containers']],
          'scale': r['properties']['template']['scale']} for r in rows]})
    return rows


def validate_streams(streams, rows):
    inventory = {r['name']: r['properties'] for r in rows}
    for process, revision in streams:
        if process.poll() is not None:
            state = inventory.get(revision, {})
            require(state.get('active') is False and state.get('replicas') == 0 and
                    state.get('runningState') in ('Stopped', 'Inactive', 'ScaleTo0'),
                    'Active or unknown revision log stream stopped; evidence UNKNOWN')


def provision(ctx, cloud, manifests, folder):
    require(cloud.get(ctx.group_id) is None and cloud.get(ctx.infrastructure_id) is None and cloud.get(ctx.role_id) is None,
            'Allocation already exists; do not adopt or overwrite')
    cloud.put(ctx.group_id, manifests['group'])
    cloud.put(ctx.environment_id, manifests['environment'])
    environment = wait_for(lambda: (lambda e: e if e and e.get('properties', {}).get('provisioningState') == 'Succeeded' else None)(cloud.get(ctx.environment_id)),
                           900, 'environment provisioning')
    require(environment['properties'].get('infrastructureResourceGroup') in (None, ctx.infrastructure_group), 'Unexpected infrastructure group')
    review = role_plan(ctx, environment)
    write_json(folder, 'role-plan.json', review)
    print(json.dumps({'rolePlan': review, 'reviewHash': digest(review), 'reviewMarker': 'hhc-role-review',
                      'waitSeconds': 600}), flush=True)
    return review


def run_acceptance(ctx, cloud, manifests, folder):
    require(owned(ctx, cloud, live=True) is not None, 'Run environment was not provisioned')
    review = role_plan(ctx, cloud.get(ctx.environment_id))
    require(json.loads((folder/'role-plan.json').read_text()) == review, 'Published role plan changed')
    wait_for(lambda: (owned(ctx, cloud, live=True) or {}).get('tags', {}).get('hhc-role-review') == digest(review),
             600, 'coordinator exact principal/role review')
    grant_reviewed_role(ctx, cloud, review)
    password = secrets.token_urlsafe(32)
    for key in ('pg', 'target'):
        manifests[key]['properties']['configuration']['secrets'] = [{'name': 'pg-password', 'value': password}]
    cloud.put(ctx.app_id('bible-ready-pg'), manifests['pg'])
    images = {key:{c['name']:c['image'] for c in manifests[key]['properties']['template']['containers']} for key in ('pg','target','caller')}
    pg_revision, pg_replica = ready_revision(ctx, cloud, 'bible-ready-pg', 'baseline',images['pg'])
    pg_sql(ctx, cloud, pg_revision, pg_replica, "CREATE EXTENSION IF NOT EXISTS vector; SELECT '{\"ok\":true}';")
    cloud.put(ctx.app_id('bible-ready-target'), manifests['target'])
    baseline, baseline_replica = ready_revision(ctx, cloud, 'bible-ready-target', 'baseline',images['target'])
    seed = "INSERT INTO versions(code,name,locale) VALUES('KJV','Isolation fixture','en'); SELECT json_build_object('history',(SELECT count(*) FROM migrations));"
    require(pg_sql(ctx, cloud, pg_revision, pg_replica, seed).get('history') == 6, 'Unexpected actual migration history')
    cloud.put(ctx.app_id('bible-ready-caller'), manifests['caller'])
    caller, caller_replica = ready_revision(ctx, cloud, 'bible-ready-caller', 'baseline',images['caller'])
    processes, handles, threads, log_files, streams = [], [], [], [], []

    def logs(revision, replica, container):
        path = folder/(revision+'-'+container+'.log')
        handle = path.open('w'); handles.append(handle)
        process = subprocess.Popen(['az', 'containerapp', 'logs', 'show', '-g', ctx.group, '-n', 'bible-ready-target',
                    '--revision', revision, '--replica', replica, '--container', container,
                    '--format', 'json', '--tail', '300', '--follow', 'true', '--only-show-errors'],
                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, start_new_session=True)
        def collect():
            for line in process.stdout:
                if handle.tell() >= 20_000_000:
                    return
                handle.write(line.replace(password, '[REDACTED]'))
                handle.flush()
        thread = threading.Thread(target=collect, daemon=True)
        thread.start(); threads.append(thread)
        processes.append(process)
        streams.append((process, revision))
        if container == 'bible-api':
            log_files.append((path, revision))
        return path

    def control(revision, replica, action):
        return cloud.control('bible-ready-target', revision, replica, action)

    def inventory(name, allowed):
        rows = runtime_inventory(ctx, cloud, folder, name, allowed)
        validate_streams(streams, rows)
        require(all(h.tell() < 20_000_000 for h in handles), 'Log evidence limit exceeded')

    def phase(name, seconds, allowed):
        inventory(name+'-start', allowed)
        cloud.control('bible-ready-caller', caller, caller_replica, 'phase-'+name)
        time.sleep(seconds+3)
        inventory(name+'-end', allowed)

    expected = {}
    fatal_paths = []
    history = {}
    try:
        logs(baseline, baseline_replica, 'bible-api')
        phase('baseline', 60, {baseline})
        expected['baseline'] = {'seconds': 60, 'revision': baseline}
        require(pg_sql(ctx, cloud, pg_revision, pg_replica,
                "SELECT json_build_object('count',count(*)) FROM pg_stat_activity WHERE application_name='isolation_baseline';")['count'] > 0,
                'PG application_name attribution unavailable')
        for suffix, fault_phase, serving, recovered_phase in [
                ('fault', 'fault', baseline, 'recovered'), ('fault-two', 'rollback_fault', 'bible-ready-target--fault', 'rollback')]:
            prepare_phase = 'prepare_fault' if suffix == 'fault' else 'prepare_rollback_fault'
            cloud.control('bible-ready-caller', caller, caller_replica, 'phase-'+prepare_phase)
            expected[prepare_phase] = {'seconds': 0, 'revision': serving}
            current = cloud.get(ctx.app_id('bible-ready-target'))['properties']
            require(current['latestRevisionName'] == current['latestReadyRevisionName'] == serving,
                    'Another revision superseded the serving baseline')
            inventory(suffix+'-preflight', {serving})
            target = manifests['target']
            target['properties']['template']['revisionSuffix'] = suffix
            containers = target['properties']['template']['containers']
            containers[0]['env'][-1]['value'] = 'isolation_'+suffix.replace('-', '_')
            containers[1]['env'][0]['value'] = 'hold'
            cloud.put(ctx.app_id('bible-ready-target'), target)
            candidate = 'bible-ready-target--'+suffix
            replica = wait_for(lambda: _candidate_replica(ctx, cloud, candidate), 300, 'candidate replica')
            logs(candidate, replica, 'bible-api')
            fatal_paths.append(logs(candidate, replica, 'daprd'))
            def armed():
                state = control(candidate, replica, 'state')
                require(not state.get('problem'), 'Candidate fault capability UNKNOWN')
                rows = state.get('observations') or []
                return state.get('armed') and state.get('held') and rows and rows[-1]['health'] == 200 and rows[-1]['ready'] == 503
            wait_for(armed, 90, 'candidate-only fault')
            require(cloud.get(ctx.app_id('bible-ready-target'))['properties']['latestReadyRevisionName'] == serving,
                    'Old revision lost authority before fault window')
            control(candidate, replica, 'phase-'+fault_phase)
            phase(fault_phase, 600, {serving, candidate})
            state = control(candidate, replica, 'state')
            history[fault_phase] = state
            require(fault_window(state.get('observations') or [], fault_phase, 600), 'Fault window not proven; UNKNOWN')
            expected[fault_phase] = {'seconds': 600, 'revision': serving}
            transition = 'recover_transition' if suffix == 'fault' else 'rollback_transition'
            recovered = candidate if suffix == 'fault' else 'bible-ready-target--restore'
            expected[transition] = {'seconds': 0, 'revisions': [serving, recovered]}
            cloud.control('bible-ready-caller', caller, caller_replica, 'phase-'+transition)
            if suffix == 'fault':
                control(candidate, replica, 'release')
                ready_revision(ctx, cloud, 'bible-ready-target', suffix,images['target'])
                recovered = candidate
            else:
                current = cloud.get(ctx.app_id('bible-ready-target'))['properties']
                require(current['latestRevisionName'] == candidate and current['latestReadyRevisionName'] == serving,
                        'Rollback no longer owns candidate')
                inventory('rollback-preflight', {serving, candidate})
                cloud.cli('containerapp', 'revision', 'copy', '-g', ctx.group, '-n', 'bible-ready-target',
                          '--from-revision', baseline, '--revision-suffix', 'restore')
                recovered, restore_replica = ready_revision(ctx, cloud, 'bible-ready-target', 'restore',images['target'])
                logs(recovered, restore_replica, 'bible-api')
                original = cloud.cli('containerapp', 'revision', 'show', '-g', ctx.group, '-n', 'bible-ready-target', '--revision', baseline)['properties']['template']
                restored = cloud.cli('containerapp', 'revision', 'show', '-g', ctx.group, '-n', 'bible-ready-target', '--revision', recovered)['properties']['template']
                original.pop('revisionSuffix', None); restored.pop('revisionSuffix', None)
                require(original == restored, 'Rollback original template/digest mismatch')
            phase(recovered_phase, 60, {serving, candidate, recovered})
            expected[recovered_phase] = {'seconds': 60, 'revision': recovered}
            old_name = 'isolation_baseline' if suffix == 'fault' else 'isolation_fault'
            wait_for(lambda: pg_sql(ctx, cloud, pg_revision, pg_replica,
                     "SELECT json_build_object('count',count(*)) FROM pg_stat_activity WHERE application_name='"+old_name+"';")['count'] == 0,
                     120, 'old process connection release')
        cloud.control('bible-ready-caller', caller, caller_replica, 'phase-stop')
        inventory('complete', {recovered})
        samples = cloud.control('bible-ready-caller', caller, caller_replica, 'state')
        require(not samples.get('problem'), 'Caller evidence incomplete')
        write_json(folder, 'caller.json', samples)
        write_json(folder, 'fault-observations.json', history)
    finally:
        stopped_early = any(p.poll() is not None for p in processes)
        for p in processes:
            if p.poll() is None:
                os.killpg(p.pid, signal.SIGTERM)
            p.wait(timeout=15)
        for thread in threads:
            thread.join(timeout=5)
        require(not any(thread.is_alive() for thread in threads), 'Log collector did not close')
        for h in handles:
            h.close()
        write_json(folder, 'log-stream-status.json', {'stoppedEarly': stopped_early,
                   'retiredStreamPolicy': 'only confirmed inactive zero-replica stopped revisions may close'})
    bind = len(fatal_paths) == 2 and all(bind_fatal(p) for p in fatal_paths)
    result = assess(samples.get('samples') or [], nonce_routes(log_files), expected, bind)
    write_json(folder, 'acceptance.json', {'result': result, 'phases': expected, 'actualManagedBindFailure': bind})
    require(result == 'PASS', 'ACA acceptance '+result)


def _candidate_replica(ctx, cloud, revision):
    rows = cloud.cli('containerapp', 'replica', 'list', '-g', ctx.group, '-n', 'bible-ready-target', '--revision', revision)
    require(len(rows) <= 1, 'Unexpected scale; acceptance UNKNOWN')
    return rows[0]['name'] if rows else None


def main():
    require(len(sys.argv) == 2 and sys.argv[1] in ('preview', 'resolve', 'rates', 'preflight', 'build-helper', 'cleanup-helper', 'provision', 'execute', 'cleanup'), 'Unexpected operation')
    ctx = Context(os.environ)
    folder = Path(os.environ.get('ISOLATION_OUTPUT', 'isolation-evidence'))
    folder.mkdir(parents=True, exist_ok=True)
    cloud = Cloud(ctx)
    if sys.argv[1] == 'rates':
        rates(folder)
        return
    if sys.argv[1] == 'resolve':
        resolve_release(ctx, folder)
        return
    if sys.argv[1] == 'preflight':
        validate_approval(ctx, os.environ)
        return
    if sys.argv[1] == 'build-helper':
        build_helper(ctx, folder)
        return
    if sys.argv[1] == 'cleanup-helper':
        cleanup_attempts(os.environ, folder, helper=True)
        return
    if sys.argv[1] == 'cleanup':
        cleanup_attempts(os.environ, folder)
        return
    manifests = plan(ctx, os.environ['BIBLE_IMAGE_REF'], os.environ['HELPER_IMAGE_REF'])
    write_json(folder, 'preview.json', manifests)
    if sys.argv[1] == 'preview':
        return
    validate_approval(ctx, os.environ)
    if sys.argv[1] == 'provision':
        provision(ctx, cloud, manifests, folder)
        return
    run_acceptance(ctx, cloud, manifests, folder)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, RuntimeError, KeyError, subprocess.TimeoutExpired) as exc:
        # Static error reasons only; no CLI output, resource payload, or fixture secrets.
        folder = Path(os.environ.get('ISOLATION_OUTPUT', 'isolation-evidence'))
        folder.mkdir(parents=True, exist_ok=True)
        write_json(folder, 'stopped.json', {'result': 'UNKNOWN', 'reason': str(exc)})
        print('Isolation stopped: '+str(exc), file=sys.stderr)
        sys.exit(1)
