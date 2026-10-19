#!/usr/bin/env python3
"""Fixed disposable ACA acceptance targets; never a production deploy command."""
import base64
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shlex
import subprocess
import sys
import time
import uuid

SUBSCRIPTION = '3765a72c-2c30-492c-8b88-7a26522903f8'
PG_IMAGE = 'pgvector/pgvector:0.8.1-pg17@sha256:3e8b3adfd27b5707128f60956f62a793c3c9326ea8cfaf0eab7adccb5d700b21'
OWNER = 'bible-readiness-isolation'
APPS = ('bible-ready-target', 'bible-ready-pg', 'bible-ready-caller')


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
           {'name': 'POSTGRES_SSLMODE', 'value': 'disable'}, {'name': 'PGAPPNAME', 'value': 'isolation_baseline'}]
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
    if not bind_failure or not samples:
        return 'UNKNOWN'
    for phase, expected in phases.items():
        records = [s for s in samples if s.get('phase') == phase]
        if len(records) < 2 or records[-1]['mono'] - records[0]['mono'] < expected['seconds']:
            return 'UNKNOWN'
        if any(b['mono'] - a['mono'] > 3 or b['mono'] < a['mono'] for a, b in zip(records, records[1:])):
            return 'UNKNOWN'
        for sample in records:
            actual = routes.get(sample['nonce'])
            if not actual:
                return 'UNKNOWN'
            if actual != {expected['revision']}:
                return 'FAIL'
    return 'PASS'


def console_result(code, output, nonce):
    frames = re.findall(r'ISOLATION_' + re.escape(nonce) + r'=([A-Za-z0-9+/=]+)', output)
    if code or len(frames) != 1 or len(frames[0]) > 8_000_000:
        raise RuntimeError('Management console evidence UNKNOWN')
    try:
        return json.loads(base64.b64decode(frames[0], validate=True))
    except (ValueError, json.JSONDecodeError) as exc:
        raise RuntimeError('Management console evidence UNKNOWN') from exc
