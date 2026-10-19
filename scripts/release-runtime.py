#!/usr/bin/env python3
"""Image-only ACA release gates. Never print or save a revision's environment values."""
import copy
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time


def require(condition, message):
    if not condition:
        raise ValueError(message)


def az(*args):
    try:
        result = subprocess.run(['az', *args, '--only-show-errors', '--output', 'json'],
                                capture_output=True, text=True, timeout=90)
        require(result.returncode == 0, 'Azure management result UNKNOWN; no health conclusion')
        return json.loads(result.stdout)
    except (subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
        raise ValueError('Azure management result UNKNOWN; no health conclusion') from exc


def target():
    return ['-g', os.environ['RESOURCE_GROUP'], '-n', os.environ['API_APP_NAME']]


def app():
    result = az('containerapp', 'show', *target())['properties']
    config = result['configuration']
    require(config['activeRevisionsMode'] == 'Single', 'Expected Single revision mode')
    dapr = config['dapr']
    require(dapr['enabled'] is True and dapr['appId'] == os.environ['DAPR_APP_ID'] and dapr['appPort'] == 9999,
            'Unexpected Dapr identity')
    return result


def revision(name):
    require(re.fullmatch(r'[a-z0-9-]+--[a-z0-9-]+', name) is not None, 'Invalid revision name')
    result = az('containerapp', 'revision', 'show', *target(), '--revision', name)
    require(result['name'] == name, 'Wrong revision identity')
    return result['properties']


def immutable(image):
    prefix = 'alive.azurecr.io/alive/bible-api'
    if re.fullmatch(re.escape(prefix) + r'@sha256:[0-9a-f]{64}', image):
        return image
    require(re.fullmatch(re.escape(prefix) + r':[A-Za-z0-9_.-]+', image) is not None, 'Unexpected image repository')
    result = az('acr', 'repository', 'show', '--name', 'alive', '--image', image.split('/', 1)[1])
    digest = result['digest']
    require(re.fullmatch(r'sha256:[0-9a-f]{64}', digest) is not None, 'Invalid registry digest')
    return prefix + '@' + digest


def template_hash(template):
    template = copy.deepcopy(template)
    template.pop('revisionSuffix', None)
    for container in template['containers']:
        container.pop('image', None)
    return hashlib.sha256(json.dumps(template, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def path(template):
    containers = template['containers']
    require(len(containers) == 1, 'Expected one application container')
    probes = [p for p in containers[0]['probes'] if p['type'] == 'Readiness']
    require(len(probes) == 1, 'Expected one readiness probe')
    value = probes[0]['httpGet']['path']
    require(value in ('/health', '/ready'), 'Unsupported rollback readiness path')
    return value


def exported(**values):
    with open(os.environ['GITHUB_ENV'], 'a') as output:
        for key, value in values.items():
            require('\n' not in str(value) and '\r' not in str(value), 'Invalid release output')
            output.write(f'{key}={value}\n')


def check_budget(template, image, allowed_revisions):
    # A reviewed ceiling is necessary: observed active_connections is not a configured budget.
    review = json.loads(os.environ['BIBLE_RELEASE_REVIEW'])
    commit = review['baselineCommit']
    require(re.fullmatch(r'[0-9a-f]{40}', commit) is not None, 'Unknown baseline source commit')
    compatibility = subprocess.run(['git', 'diff', '--quiet', commit, 'HEAD', '--',
                                    'migrations', 'internal/models/models.go', 'internal/database/database.go'],
                                   capture_output=True, timeout=15)
    require(compatibility.returncode == 0, 'Migration/model/DB startup changed; separate rollback review required')
    now = datetime.datetime.now(datetime.timezone.utc)
    at = datetime.datetime.fromisoformat(review['verifiedAt'])
    require(at.tzinfo is not None and datetime.timedelta(0) <= now-at <= datetime.timedelta(hours=6),
            'PG release review missing, stale, or future-dated')
    require(review['evidence'] and review['baselineImage'] == image and review['baselinePoolMaxOpen'] == 8,
            'Rollback image pool is not certified by this review')
    require(review['importsFrozen'] is True, 'Import freeze/retirement not reviewed')
    require(review['ordinarySlots'] == 844, 'Refresh the reviewed PG capacity before rollout')
    for env in template['containers'][0].get('env', []):
        limits = {'DB_MAX_OPEN_CONNS': '8', 'DB_MAX_IDLE_CONNS': '2',
                  'DB_CONN_MAX_LIFETIME': '30m', 'DB_CONN_MAX_IDLE_TIME': '5m'}
        if env['name'] in limits:
            require(env.get('value') == limits[env['name']], 'Unexpected pool override or secret reference')
    max_replicas = template['scale']['maxReplicas']
    require(type(max_replicas) is int and max_replicas >= 1, 'Unknown replica ceiling')
    other = review['otherConnectionsCeiling']
    reserve = review.get('reserveConnections', 100)
    require(type(reserve) is int and reserve >= 32, 'Reviewed platform reserve must be at least 32')
    require(type(other) is int and other >= 0 and other + 3*max_replicas*8 + reserve <= 844,
            'Insufficient configured connection budget for old/candidate/rollback and reserve')
    jobs = az('containerapp', 'job', 'list', '-g', os.environ['RESOURCE_GROUP'])
    if any(job['name'] == 'bible-import' for job in jobs):
        executions = az('containerapp', 'job', 'execution', 'list', '-g', os.environ['RESOURCE_GROUP'], '-n', 'bible-import')
        require(all(e['properties']['status'] in ('Succeeded', 'Failed', 'Stopped') for e in executions),
                'Import running or execution state unknown')
    # Count still-running inactive revisions too; active=false does not release a pool.
    revisions = az('containerapp', 'revision', 'list', *target(), '--all')
    live = [r for r in revisions if r['properties'].get('active') is True or
            r['properties'].get('replicas', 0) > 0 or
            r['properties'].get('runningState') not in ('Stopped', 'Inactive', 'ScaleTo0')]
    live_names = {r['name'] for r in live}
    require(live_names <= allowed_revisions, 'Another revision may still hold connections; wait for quiet window')
    return live_names


def capture():
    current = app()
    name = current['latestReadyRevisionName']
    baseline = revision(name)
    require(baseline['active'] is True and baseline['healthState'] == 'Healthy' and
            baseline['runningState'] in ('Running', 'RunningAtMaxScale'), 'Rollback baseline is not active and healthy')
    require(current['latestRevisionName'] == name, 'An unfinished rollout is already present')
    template = baseline['template']
    health_path = path(template)
    image = immutable(template['containers'][0]['image'])
    require(check_budget(template, image, {name}) == {name}, 'Baseline is missing from revision inventory')
    snapshot = Path(os.environ['RUNNER_TEMP']) / 'bible-rollback-snapshot.json'
    snapshot.write_text(json.dumps({'revision': name, 'image': image, 'readinessPath': health_path,
                                    'templateHash': template_hash(template)}))
    exported(PREVIOUS_READY_REVISION=name, PREVIOUS_IMAGE_REF=image, PREVIOUS_READINESS_PATH=health_path,
             BIBLE_ROLLBACK_SNAPSHOT=str(snapshot))
    print('Captured healthy rollback baseline and reviewed overlap budget')


def snapshot():
    data = json.loads(Path(os.environ['BIBLE_ROLLBACK_SNAPSHOT']).read_text())
    require(data['revision'] == os.environ['PREVIOUS_READY_REVISION'] and data['image'] == os.environ['PREVIOUS_IMAGE_REF']
            and data['readinessPath'] == os.environ['PREVIOUS_READINESS_PATH'], 'Rollback snapshot identity changed')
    require(template_hash(revision(data['revision'])['template']) == data['templateHash'], 'Original template changed')
    return data


def update():
    data = snapshot()
    baseline = revision(data['revision'])
    require(check_budget(baseline['template'], data['image'], {data['revision']}) == {data['revision']}, 'Baseline is missing from revision inventory')
    require(baseline['active'] is True and baseline['healthState'] == 'Healthy' and baseline['runningState'] in ('Running', 'RunningAtMaxScale'), 'Baseline lost health before update')
    current = app()
    require(current['latestReadyRevisionName'] == current['latestRevisionName'] == data['revision'], 'Baseline changed before update')
    image = os.environ['IMAGE_REF']
    require(immutable(image) == image, 'Candidate must use an immutable image')
    suffix = f"release-{os.environ['GITHUB_RUN_ID']}-{os.environ['GITHUB_RUN_ATTEMPT']}"
    require(re.fullmatch(r'release-[0-9]+-[0-9]+', suffix) is not None, 'Invalid release identity')
    candidate = os.environ['API_APP_NAME'] + '--' + suffix
    # Persist ownership before a possibly ambiguous network result from the mutation.
    exported(CANDIDATE_REVISION=candidate, RUNTIME_UPDATE_STARTED='true')
    result = az('containerapp', 'update', *target(), '--image', image, '--revision-suffix', suffix)
    require(result['properties']['latestRevisionName'] == candidate, 'Image update returned another candidate')


def wait(name, image, expected_hash=None):
    deadline = time.monotonic() + 600
    for attempt in range(60):
        require(time.monotonic() < deadline, 'Candidate readiness deadline exceeded')
        current = app()
        require(current['latestRevisionName'] == name, 'Candidate superseded; refusing stale success')
        state = revision(name)
        require(state['template']['containers'][0]['image'] == image, 'Candidate image digest mismatch')
        if expected_hash is not None:
            require(template_hash(state['template']) == expected_hash, 'Rollback template/probe mismatch')
        if state['healthState'] == 'Unhealthy' or state['runningState'] in ('Failed', 'Degraded', 'Stopped'):
            raise ValueError('Candidate failed readiness')
        if (current['latestReadyRevisionName'] == name and state['active'] is True and
                state['healthState'] == 'Healthy' and state['runningState'] in ('Running', 'RunningAtMaxScale')):
            replicas = az('containerapp', 'replica', 'list', *target(), '--revision', name)
            containers = [c for r in replicas for c in r['properties']['containers']]
            if containers and all(c.get('ready') is True for c in containers):
                print('Verified pinned revision, digest and available replica readiness')
                return
        if attempt < 59:
            time.sleep(10)
    raise ValueError('Candidate readiness deadline exceeded')


def rollback():
    require(os.environ.get('RUNTIME_UPDATE_STARTED') == 'true', 'No runtime mutation attempted; preserve healthy baseline')
    data = snapshot()
    candidate = os.environ['CANDIDATE_REVISION']
    current = app()
    require(current['latestRevisionName'] == candidate and current['latestReadyRevisionName'] in (candidate, data['revision']),
            'Rollback ownership UNKNOWN or superseded; refusing another revision')
    state = revision(candidate)
    require(state['template']['containers'][0]['image'] == os.environ['IMAGE_REF'] and
            template_hash(state['template']) == data['templateHash'], 'Candidate ownership/template mismatch')
    check_budget(revision(data['revision'])['template'], data['image'], {candidate, data['revision']})
    suffix = f"rollback-{os.environ['GITHUB_RUN_ID']}-{os.environ['GITHUB_RUN_ATTEMPT']}"
    result = az('containerapp', 'revision', 'copy', *target(), '--from-revision', data['revision'],
                '--image', data['image'], '--revision-suffix', suffix)
    name = os.environ['API_APP_NAME'] + '--' + suffix
    require(result['properties']['latestRevisionName'] == name, 'Unknown rollback candidate')
    exported(ROLLBACK_REVISION=name)
    wait(name, data['image'], data['templateHash'])


if __name__ == '__main__':
    try:
        command = sys.argv[1]
        if command == 'capture': capture()
        elif command == 'update': update()
        elif command == 'wait': wait(os.environ['CANDIDATE_REVISION'], os.environ['IMAGE_REF'], snapshot()['templateHash'])
        elif command == 'rollback': rollback()
        else: raise ValueError('Unknown release operation')
    except (ValueError, KeyError, OSError, TypeError) as exc:
        # Azure JSON and env can contain secrets; never dump response bodies or raw exceptions.
        print('Release gate failed: ' + (str(exc) if type(exc) is ValueError else type(exc).__name__), file=sys.stderr)
        sys.exit(1)
