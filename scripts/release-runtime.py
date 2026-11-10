#!/usr/bin/env python3
"""Pinned Bible runtime release gates. Never print or save environment values."""
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
from urllib.request import urlopen

# One reviewed startup-only transition. New startup changes require a separate source review.
STARTUP_LOCK_TRANSITION = {
    'baselineSha256': 'ff36bf0c6e95fdcdd7b0601a6173ff89548196049290a4bcac6b14e3bc4e0426',
    'candidateSha256': 'f617cca0ebca03985773628edae32ec52203460f67d3d9bd68237d9d2b570eca',
    'rollbackPolicy': 'forward-only',
}

RUNTIME = json.loads((Path(__file__).resolve().parent.parent / 'infra/runtime.json').read_text())


def require(condition, message):
    if not condition:
        raise ValueError(message)


def az(*args, body=None):
    try:
        result = subprocess.run(['az', *args, '--only-show-errors', '--output', 'json'],
                                capture_output=True, text=True, timeout=90,
                                input=json.dumps(body) if body is not None else None)
        require(result.returncode == 0, 'Azure management result UNKNOWN; no health conclusion')
        return json.loads(result.stdout or ('{}' if body is not None else ''))
    except (subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
        raise ValueError('Azure management result UNKNOWN; no health conclusion') from exc


def target():
    return ['-g', os.environ['RESOURCE_GROUP'], '-n', os.environ['API_APP_NAME']]


def app():
    result = az('containerapp', 'show', *target())['properties']
    config = result['configuration']
    require(config['activeRevisionsMode'] == 'Single', 'Expected Single revision mode')
    dapr = config.get('dapr') or {}
    if dapr.get('enabled') is True:
        require(dapr.get('appId') == 'bible-api' and dapr.get('appPort') == 9999, 'Unexpected Dapr identity')
    ingress = config.get('ingress')
    if ingress is not None:
        require(ingress.get('external') is False and ingress.get('targetPort') == 9999 and
                ingress.get('allowInsecure') is False and not ingress.get('customDomains') and
                not ingress.get('additionalPortMappings'), 'Unexpected Bible ingress exposure')
    require(dapr.get('enabled') is True or ingress is not None, 'No Bible invocation route')
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
    # Revision readback uses null for these platform defaults, even when explicitly set.
    for field, default in (('cooldownPeriod', 300), ('pollingInterval', 30)):
        if template['scale'].get(field) is None:
            template['scale'][field] = default
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


def ready_template(template):
    path(template)
    result = copy.deepcopy(template)
    for probe in result['containers'][0]['probes']:
        if probe['type'] == 'Readiness':
            probe['httpGet']['path'] = '/ready'
    return result


def candidate_template(template):
    result = ready_template(template)
    env = result['containers'][0].setdefault('env', [])
    existing = [{k: v for k, v in e.items() if v not in (None, '')}
                for e in env if e['name'] == 'BIBLE_GATEWAY_TOKEN']
    require(not existing or existing == [{'name': 'BIBLE_GATEWAY_TOKEN', 'secretRef': 'bible-gateway-token'}],
            'Unexpected Bible gateway credential binding')
    if not existing:
        env.append({'name': 'BIBLE_GATEWAY_TOKEN', 'secretRef': 'bible-gateway-token'})
    result['scale'] = copy.deepcopy(RUNTIME['scale'])
    return result


def transport(configuration):
    result = {k: copy.deepcopy(configuration.get(k)) for k in ('ingress', 'dapr')}
    if result['ingress'] is not None:
        result['ingress'].pop('fqdn', None)
        # CLI readback includes this null field, unsupported by the pinned write API.
        require(result['ingress'].get('targetPortHttpScheme') is None,
                'Review non-default ingress targetPortHttpScheme before release')
        result['ingress'].pop('targetPortHttpScheme', None)
    return result


def candidate_configuration(configuration):
    result = transport(configuration)
    result['ingress'] = copy.deepcopy(RUNTIME['ingress'])
    result['dapr'] = {**(result['dapr'] or {}), 'enabled': RUNTIME['daprEnabled']}
    return result


def configuration_matches(actual, expected):
    for key in ('ingress', 'dapr'):
        value = expected.get(key)
        if value is None:
            if actual.get(key) is not None:
                return False
        else:
            for field, wanted in value.items():
                observed = (actual.get(key) or {}).get(field)
                if field == 'transport' and isinstance(observed, str):
                    observed, wanted = observed.lower(), wanted.lower()
                if observed != wanted:
                    return False
    return True


def runtime_url():
    subscription = os.environ['AZURE_SUBSCRIPTION_ID']
    require(re.fullmatch(r'[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}', subscription) is not None,
            'Invalid subscription identity')
    require(all(re.fullmatch(r'[a-z0-9-]+', os.environ[key]) for key in ('RESOURCE_GROUP', 'API_APP_NAME')),
            'Invalid application target')
    return ('https://management.azure.com/subscriptions/'+subscription+'/resourceGroups/'+os.environ['RESOURCE_GROUP']+
           '/providers/Microsoft.App/containerApps/'+os.environ['API_APP_NAME']+'?api-version=2026-01-01')


def patch_runtime(template, configuration):
    return az('rest', '--method', 'PATCH', '--url', runtime_url(), '--body', '@/dev/stdin',
              body={'properties': {'template': template, 'configuration': configuration}})


def check_gateway_http():
    if RUNTIME['daprEnabled'] and RUNTIME['scale']['minReplicas'] > 0:
        return
    review = json.loads(os.environ['BIBLE_RELEASE_REVIEW'])
    require(review.get('gatewayHttpVerified') is True, 'Verified Gateway HTTP cutover is required')
    require(os.environ['API_GATEWAY_APP_NAME'] == 'api-gateway', 'Unexpected Gateway target')
    gateway = az('containerapp', 'show', '-g', os.environ['RESOURCE_GROUP'], '-n', 'api-gateway')['properties']
    require(gateway['latestRevisionName'] == gateway['latestReadyRevisionName'] == review.get('gatewayHttpRevision') and
            gateway['template']['containers'][0]['image'] == review.get('gatewayHttpImage'),
            'Gateway HTTP evidence no longer matches the live revision/image')


def configure():
    data = snapshot()
    current = app()
    desired = candidate_configuration(data['configuration'])
    require(current['latestRevisionName'] == current['latestReadyRevisionName'] == os.environ['CANDIDATE_REVISION'],
            'Authenticate the ready image before enabling ingress')
    state = revision(os.environ['CANDIDATE_REVISION'])
    require(state['active'] is True and state['healthState'] == 'Healthy' and
            state['runningState'] in ('Running', 'RunningAtMaxScale') and
            state['template']['containers'][0]['image'] == os.environ['IMAGE_REF'] and
            template_hash(state['template']) == data['candidateTemplateHash'],
            'Authenticated image lost readiness or changed before configuring ingress')
    require(transport(current['configuration']) == data['configuration'], 'Transport changed before configuration')
    check_gateway_http()
    exported(CONFIGURATION_UPDATE_STARTED='true')
    az('rest', '--method', 'PATCH', '--url', runtime_url(), '--body', '@/dev/stdin',
       body={'properties': {'configuration': desired}})
    deadline = time.monotonic() + 180
    for attempt in range(30):
        require(time.monotonic() < deadline, 'Transport configuration deadline exceeded')
        current = app()
        require(current['latestRevisionName'] == os.environ['CANDIDATE_REVISION'],
                'Transport update superseded; refusing another rollout')
        if configuration_matches(current['configuration'], desired):
            return
        require(configuration_matches(current['configuration'], data['configuration']),
                'Unexpected transport change during configuration')
        if attempt < 29:
            time.sleep(2)
    raise ValueError('Transport configuration deadline exceeded')


def exported(**values):
    with open(os.environ['GITHUB_ENV'], 'a') as output:
        for key, value in values.items():
            require('\n' not in str(value) and '\r' not in str(value), 'Invalid release output')
            output.write(f'{key}={value}\n')


def startup_transition(review):
    commit = review['baselineCommit']
    require(re.fullmatch(r'[0-9a-f]{40}', commit) is not None, 'Unknown baseline source commit')
    compatibility = subprocess.run(['git', 'diff', '--quiet', commit, 'HEAD', '--',
                                    'migrations', 'internal/models/models.go'],
                                   capture_output=True, timeout=15)
    require(compatibility.returncode == 0, 'Migration/model changed; separate rollback review required')
    hashes = []
    for ref in (commit, 'HEAD'):
        source = subprocess.run(['git', 'show', ref + ':internal/database/database.go'],
                                capture_output=True, timeout=15)
        require(source.returncode == 0, 'Unknown database startup source')
        hashes.append(hashlib.sha256(source.stdout).hexdigest())
    if hashes[0] == hashes[1]:
        require(review.get('startupTransition') is None, 'Stale startup transition review on unchanged source')
        return None
    expected = dict(STARTUP_LOCK_TRANSITION)
    require(hashes == [expected['baselineSha256'], expected['candidateSha256']] and
            review.get('startupTransition') == expected,
            'DB startup changed without exact reviewed forward-only transition')
    return expected


def check_release_boundaries(template, image, allowed_revisions):
    review = json.loads(os.environ['BIBLE_RELEASE_REVIEW'])
    startup_transition(review)
    now = datetime.datetime.now(datetime.timezone.utc)
    at = datetime.datetime.fromisoformat(review['verifiedAt'])
    require(at.tzinfo is not None and datetime.timedelta(0) <= now-at <= datetime.timedelta(hours=6),
            'PG release review missing, stale, or future-dated')
    require(review['evidence'] and review['baselineImage'] == image and review['baselinePoolMaxOpen'] == 8,
            'Rollback image pool is not certified by this review')
    require(review['importsFrozen'] is True, 'Import freeze/retirement not reviewed')
    for env in template['containers'][0].get('env', []):
        limits = {'DB_MAX_OPEN_CONNS': '8', 'DB_MAX_IDLE_CONNS': '2',
                  'DB_CONN_MAX_LIFETIME': '30m', 'DB_CONN_MAX_IDLE_TIME': '5m'}
        if env['name'] in limits:
            require(env.get('value') == limits[env['name']], 'Unexpected pool override or secret reference')
    scale = template['scale']
    rules = copy.deepcopy(scale.get('rules', []))
    for rule in rules:
        if rule.get('custom', {}).get('auth') == []:
            rule['custom'].pop('auth')
    cpu = {'name': 'cpu-scaler', 'custom': {'type': 'cpu', 'metadata': {'type': 'Utilization', 'value': '70'}}}
    http = {'name': 'http-concurrency', 'http': {'metadata': {'concurrentRequests': '20'}}}
    require(scale.get('minReplicas') in (0, 1) and scale.get('maxReplicas') == 8 and
            (rules == [cpu, http] or (rules == [cpu] and scale.get('minReplicas') == 1)),
            'Unexpected Bible scaling or missing zero-activation trigger')
    jobs = az('containerapp', 'job', 'list', '-g', os.environ['RESOURCE_GROUP'])
    if any(job['name'] == 'bible-import' for job in jobs):
        executions = az('containerapp', 'job', 'execution', 'list', '-g', os.environ['RESOURCE_GROUP'], '-n', 'bible-import')
        require(all(e['properties']['status'] in ('Succeeded', 'Failed', 'Stopped') for e in executions),
                'Import running or execution state unknown')
    # Count still-running inactive revisions too; active=false does not release a pool.
    revisions = az('containerapp', 'revision', 'list', *target(), '--all')
    live = [r for r in revisions if r['properties'].get('active') is True or
            r['properties'].get('replicas', 0) > 0 or
            r['properties'].get('runningState') not in ('Stopped', 'Inactive', 'ScaleTo0', 'ScaledToZero')]
    live_names = {r['name'] for r in live}
    require(live_names <= allowed_revisions, 'Another revision may still hold connections; wait for quiet window')
    return live_names


def running_baseline(current, name, baseline):
    require(current['latestRevisionName'] == current['latestReadyRevisionName'] == name,
            'Baseline changed before activation')
    if baseline['runningState'] in ('ScaleTo0', 'ScaledToZero') and baseline['template']['scale'].get('minReplicas') == 0:
        require(current['configuration'].get('ingress'), 'No HTTP wake route for the baseline')
        # Wake via the real caller path; metadata reads alone cannot start a zero-scaled app.
        with urlopen('https://www.alive.org.tw/api/bible/v2/versions', timeout=120) as response:
            require(response.status == 200 and isinstance(json.loads(response.read(65537)), list),
                    'Baseline HTTP activation failed')
        for attempt in range(12):
            current = app()
            require(current['latestRevisionName'] == current['latestReadyRevisionName'] == name,
                    'Baseline was superseded during activation')
            baseline = revision(name)
            if baseline['runningState'] in ('Running', 'RunningAtMaxScale'):
                break
            if attempt < 11:
                time.sleep(5)
    return current, baseline


def capture():
    current = app()
    name = current['latestReadyRevisionName']
    require(current['latestRevisionName'] == name, 'An unfinished rollout is already present')
    baseline = revision(name)
    current, baseline = running_baseline(current, name, baseline)
    require(baseline['active'] is True and baseline['healthState'] == 'Healthy' and
            baseline['runningState'] in ('Running', 'RunningAtMaxScale'), 'Rollback baseline is not active and healthy')
    require(current['latestRevisionName'] == name, 'An unfinished rollout is already present')
    template = baseline['template']
    health_path = path(template)
    image = immutable(template['containers'][0]['image'])
    require(check_release_boundaries(template, image, {name}) == {name}, 'Baseline is missing from revision inventory')
    snapshot = Path(os.environ['RUNNER_TEMP']) / 'bible-rollback-snapshot.json'
    snapshot.write_text(json.dumps({'revision': name, 'image': image, 'readinessPath': health_path,
                                    'templateHash': template_hash(template),
                                    'configuration': transport(current['configuration']),
                                    'startupTransition': startup_transition(json.loads(os.environ['BIBLE_RELEASE_REVIEW']))}))
    exported(PREVIOUS_READY_REVISION=name, PREVIOUS_IMAGE_REF=image, PREVIOUS_READINESS_PATH=health_path,
             PREVIOUS_BIBLE_TRANSPORT='http' if current['configuration'].get('ingress') else 'dapr',
             BIBLE_ROLLBACK_SNAPSHOT=str(snapshot))
    print('Captured healthy rollback baseline and reviewed Bible release boundaries')


def snapshot():
    data = json.loads(Path(os.environ['BIBLE_ROLLBACK_SNAPSHOT']).read_text())
    require(data['revision'] == os.environ['PREVIOUS_READY_REVISION'] and data['image'] == os.environ['PREVIOUS_IMAGE_REF']
            and data['readinessPath'] == os.environ['PREVIOUS_READINESS_PATH'], 'Rollback snapshot identity changed')
    template = revision(data['revision'])['template']
    require(template_hash(template) == data['templateHash'], 'Original template changed')
    data['candidateTemplateHash'] = template_hash(candidate_template(template))
    return data


def drain_startup_predecessor(data):
    # Deactivation prevents scale/restart of the unlocked predecessor. Do not infer drain from active=false alone.
    az('containerapp', 'revision', 'deactivate', *target(), '--revision', data['revision'])
    deadline = time.monotonic() + 600
    for attempt in range(60):
        require(time.monotonic() < deadline, 'Startup predecessor drain deadline exceeded; forward-only repair required')
        current = app()
        require(current['latestRevisionName'] == data['revision'] and
                transport(current['configuration']) == data['configuration'],
                'Startup cutover superseded; forward-only repair required')
        state = revision(data['revision'])
        require(template_hash(state['template']) == data['templateHash'] and
                immutable(state['template']['containers'][0]['image']) == data['image'],
                'Startup predecessor identity changed')
        if (state.get('active') is False and state.get('replicas') == 0 and
                state.get('runningState') in ('Stopped', 'Inactive', 'ScaleTo0', 'ScaledToZero') and
                az('containerapp', 'replica', 'list', *target(), '--revision', data['revision']) == []):
            require(check_release_boundaries(state['template'], data['image'], set()) == set(),
                    'Startup predecessor has not drained')
            return
        if attempt < 59:
            time.sleep(10)
    raise ValueError('Startup predecessor drain deadline exceeded; forward-only repair required')


def update():
    data = snapshot()
    current, baseline = running_baseline(app(), data['revision'], revision(data['revision']))
    require(check_release_boundaries(baseline['template'], data['image'], {data['revision']}) == {data['revision']}, 'Baseline is missing from revision inventory')
    require(data['startupTransition'] == startup_transition(json.loads(os.environ['BIBLE_RELEASE_REVIEW'])),
            'Startup review changed after capture')
    require(baseline['active'] is True and baseline['healthState'] == 'Healthy' and baseline['runningState'] in ('Running', 'RunningAtMaxScale'), 'Baseline lost health before update')
    require(current['latestReadyRevisionName'] == current['latestRevisionName'] == data['revision'], 'Baseline changed before update')
    require(transport(current['configuration']) == data['configuration'], 'App-scope configuration changed after capture')
    require('bible-gateway-token' in {s['name'] for s in current['configuration'].get('secrets', [])},
            'Dedicated Bible gateway secret must be staged first')
    if RUNTIME['scale']['minReplicas'] == 0:
        require(current['configuration'].get('ingress') and any(
            'http' in rule for rule in baseline['template']['scale'].get('rules', [])),
            'Establish the private HTTP ingress/scaler before activating zero replicas')
    check_gateway_http()
    image = os.environ['IMAGE_REF']
    require(immutable(image) == image, 'Candidate must use an immutable image')
    suffix = f"release-{os.environ['GITHUB_RUN_ID']}-{os.environ['GITHUB_RUN_ATTEMPT']}"
    require(re.fullmatch(r'release-[0-9]+-[0-9]+', suffix) is not None, 'Invalid release identity')
    candidate = os.environ['API_APP_NAME'] + '--' + suffix
    runtime_url()
    template = candidate_template(baseline['template'])
    template['containers'][0]['image'] = image
    template['revisionSuffix'] = suffix
    # JSON Merge Patch preserves configuration/secrets; stdin avoids saving environment values or reading secrets via YAML.
    # Persist ownership before a possibly ambiguous network result from the mutation.
    exported(CANDIDATE_REVISION=candidate, RUNTIME_UPDATE_STARTED='true')
    if data['startupTransition'] is not None:
        drain_startup_predecessor(data)
    result = patch_runtime(template, data['configuration'])
    require(result.get('properties', {}).get('latestRevisionName') in (None, data['revision'], candidate),
            'Runtime update returned another candidate')


def wait(name, image, expected_hash=None, previous=None, expected_configuration=None):
    deadline = time.monotonic() + 600
    for attempt in range(60):
        require(time.monotonic() < deadline, 'Candidate readiness deadline exceeded')
        current = app()
        if previous is not None and current['latestRevisionName'] == previous:
            require(current['latestReadyRevisionName'] in (previous, os.environ.get('PREVIOUS_READY_REVISION')),
                    'Previous ready revision changed during update')
            if attempt < 59:
                time.sleep(10)
            continue
        require(current['latestRevisionName'] == name, 'Candidate superseded; refusing stale success')
        if expected_configuration is not None:
            require(configuration_matches(current['configuration'], expected_configuration), 'Runtime transport differs from reviewed candidate')
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
    require(data['startupTransition'] is None,
            'Startup cutover is forward-only; never restart the unlocked predecessor; use a reviewed compatible forward repair')
    candidate = os.environ['CANDIDATE_REVISION']
    current = app()
    require(configuration_matches(current['configuration'], candidate_configuration(data['configuration'])) or
            configuration_matches(current['configuration'], data['configuration']),
            'App-scope settings changed after update; rollback ownership unknown')
    require(current['latestRevisionName'] == candidate and current['latestReadyRevisionName'] in (candidate, data['revision']),
            'Rollback ownership UNKNOWN or superseded; refusing another revision')
    state = revision(candidate)
    require(state['template']['containers'][0]['image'] == os.environ['IMAGE_REF'] and
            template_hash(state['template']) == data['candidateTemplateHash'], 'Candidate ownership/template mismatch')
    check_release_boundaries(revision(data['revision'])['template'], data['image'], {candidate, data['revision']})
    suffix = f"rollback-{os.environ['GITHUB_RUN_ID']}-{os.environ['GITHUB_RUN_ATTEMPT']}"
    name = os.environ['API_APP_NAME'] + '--' + suffix
    template = copy.deepcopy(revision(data['revision'])['template'])
    template['containers'][0]['image'] = data['image']
    template['revisionSuffix'] = suffix
    result = patch_runtime(template, data['configuration'])
    require(result.get('properties', {}).get('latestRevisionName') in (None, candidate, name),
            'Unknown rollback candidate')
    exported(ROLLBACK_REVISION=name)
    wait(name, data['image'], data['templateHash'], previous=candidate, expected_configuration=data['configuration'])


if __name__ == '__main__':
    try:
        command = sys.argv[1]
        if command == 'capture': capture()
        elif command == 'update': update()
        elif command == 'configure': configure()
        elif command == 'wait': wait(os.environ['CANDIDATE_REVISION'], os.environ['IMAGE_REF'],
                                     snapshot()['candidateTemplateHash'], os.environ['PREVIOUS_READY_REVISION'],
                                     candidate_configuration(snapshot()['configuration']) if
                                     os.environ.get('CONFIGURATION_UPDATE_STARTED') == 'true' else snapshot()['configuration'])
        elif command == 'rollback': rollback()
        else: raise ValueError('Unknown release operation')
    except (ValueError, KeyError, OSError, TypeError) as exc:
        # Azure JSON and env can contain secrets; never dump response bodies or raw exceptions.
        print('Release gate failed: ' + (str(exc) if type(exc) is ValueError else type(exc).__name__), file=sys.stderr)
        sys.exit(1)
