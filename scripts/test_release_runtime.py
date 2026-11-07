#!/usr/bin/env python3
"""Exercise the real release CLI against a strict fake Azure boundary."""
import datetime
import contextlib
import importlib.util
import sys
from unittest import mock
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
RUNTIME = json.loads((ROOT/'infra/runtime.json').read_text())
IMAGE = 'alive.azurecr.io/alive/bible-api@sha256:' + 'a' * 64
NEW_IMAGE = 'alive.azurecr.io/alive/bible-api@sha256:' + 'b' * 64
TEMPLATE = {'containers': [{'name': 'bible-api', 'image': IMAGE, 'probes': [
    {'type': 'Readiness', 'httpGet': {'path': '/health', 'port': 9999}},
    {'type': 'Liveness', 'httpGet': {'path': '/health', 'port': 9999}}], 'env': []}],
    'scale': {'minReplicas': 1, 'maxReplicas': 8, 'rules': [{'name': 'cpu-scaler', 'custom': {'type': 'cpu', 'metadata': {'type': 'Utilization', 'value': '70'}}}]}}
TEMPLATE['scale']['rules'].append({'name':'http-concurrency','http':{'metadata':{'concurrentRequests':'20'}}})
BASE = {'name': 'bible-api--old', 'properties': {'active': True, 'healthState': 'Healthy',
    'runningState': 'RunningAtMaxScale', 'template': TEMPLATE}}
APP = {'properties': {'latestReadyRevisionName': 'bible-api--old', 'latestRevisionName': 'bible-api--old',
    'configuration': {'activeRevisionsMode': 'Single', 'ingress':RUNTIME['ingress'], 'secrets': [{'name':'bible-gateway-token'}], 'dapr': {'enabled': True, 'appId': 'bible-api', 'appPort': 9999}}}}

FAKE_AZ = '''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
p=Path(os.environ['CASE_DIR'])
args=sys.argv[1:]
with (p/'calls').open('a') as f:f.write(json.dumps(args)+'\\n')
data=json.loads((p/'azure.json').read_text())
key=' '.join(args[:3])
if key=='containerapp revision show':
    name=args[args.index('--revision')+1]
    result=data['revisions'][name]
elif key=='containerapp replica list':result=data['replicas']
elif key=='containerapp job list':result=data['jobs']
elif key=='containerapp job execution':result=data['executions']
elif args[:2]==['containerapp','show']:
    if args[args.index('-n')+1]=='api-gateway':result=data['gateway']
    else:
        if data.get('pending_configuration'):
            if data.get('pending_reads',0)>0:data['pending_reads']-=1
            else:data['app']['properties']['configuration'].update(data.pop('pending_configuration'))
            (p/'azure.json').write_text(json.dumps(data))
        result=data['app']
elif args[0]=='rest':
    payload=json.load(sys.stdin)
    (p/'patch.json').write_text(json.dumps(payload))
    if 'template' not in payload['properties']:
        (p/'configured').touch()
        if data.get('configure_pending_reads'):
            data['pending_configuration']=payload['properties']['configuration']
            data['pending_reads']=data['configure_pending_reads']
        else:data['app']['properties']['configuration'].update(payload['properties']['configuration'])
        (p/'azure.json').write_text(json.dumps(data))
        print(json.dumps(data['app']))
        sys.exit(0)
    suffix=payload['properties']['template']['revisionSuffix']
    if suffix.startswith('rollback-'):
        (p/'copied').touch();result=data['copied_app']
        name='bible-api--'+suffix
        revision=json.loads(json.dumps(data['revisions']['bible-api--old']));revision['name']=name
        revision['properties']['template']=payload['properties']['template']
        if data.get('corrupt_copy_probe'):revision['properties']['template']['containers'][0]['probes'][0]['httpGet']['path']='/ready'
        data['revisions'][name]=revision
        data['app']['properties'].update(latestRevisionName=name,latestReadyRevisionName=name)
        data['app']['properties']['configuration'].update(payload['properties']['configuration'])
        (p/'azure.json').write_text(json.dumps(data))
    else:
        (p/'updated').touch();result=data['updated_app']
elif key=='containerapp revision copy':
    (p/'copied').touch();result=data['copied_app']
    source=args[args.index('--from-revision')+1]
    name=result['properties']['latestRevisionName']
    revision=json.loads(json.dumps(data['revisions'][source]));revision['name']=name
    revision['properties']['template']['containers'][0]['image']=args[args.index('--image')+1]
    if data.get('corrupt_copy_probe'):revision['properties']['template']['containers'][0]['probes'][0]['httpGet']['path']='/ready'
    data['revisions'][name]=revision
    data['app']['properties'].update(latestRevisionName=name,latestReadyRevisionName=name)
    (p/'azure.json').write_text(json.dumps(data))
elif key=='containerapp revision list':result=[r for r in data['revisions'].values() if '--all' in args or r['properties']['active']]
elif key=='acr repository show':result=data['registry']
else:sys.exit('unexpected az command: '+repr(args))
if result=='DENIED':sys.exit(23)
print(json.dumps(result))
'''

class ReleaseRuntimeTests(unittest.TestCase):
    def test_transport_omits_unsupported_null_ingress_readback_field(self):
        spec=importlib.util.spec_from_file_location('runtime_ingress',ROOT/'scripts/release-runtime.py')
        runtime=importlib.util.module_from_spec(spec);spec.loader.exec_module(runtime)
        configuration=json.loads(json.dumps(APP['properties']['configuration']))
        configuration['ingress']['fqdn']='bible-api.internal.example'
        configuration['ingress']['targetPortHttpScheme']=None
        result=runtime.transport(configuration)
        self.assertNotIn('fqdn',result['ingress'])
        self.assertNotIn('targetPortHttpScheme',result['ingress'])
        self.assertIn('targetPortHttpScheme',configuration['ingress'])
        self.assertEqual(result['ingress'],RUNTIME['ingress'])
        configuration['ingress']['targetPortHttpScheme']='http'
        with self.assertRaisesRegex(ValueError,'non-default ingress'):
            runtime.transport(configuration)

    def test_template_hash_normalizes_only_platform_scale_defaults(self):
        spec=importlib.util.spec_from_file_location('runtime_defaults',ROOT/'scripts/release-runtime.py')
        runtime=importlib.util.module_from_spec(spec);spec.loader.exec_module(runtime)
        absent=json.loads(json.dumps(TEMPLATE))
        explicit=json.loads(json.dumps(TEMPLATE))
        explicit['scale'].update(cooldownPeriod=300,pollingInterval=30)
        nulls=json.loads(json.dumps(TEMPLATE))
        nulls['scale'].update(cooldownPeriod=None,pollingInterval=None)
        self.assertEqual(runtime.template_hash(absent),runtime.template_hash(explicit))
        self.assertEqual(runtime.template_hash(nulls),runtime.template_hash(explicit))
        for field,value in [('cooldownPeriod',301),('pollingInterval',31)]:
            changed=json.loads(json.dumps(explicit));changed['scale'][field]=value
            self.assertNotEqual(runtime.template_hash(changed),runtime.template_hash(explicit))

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.source = self.path/'source'
        self.source.mkdir()
        self.git('init','-q')
        (self.source/'migrations').mkdir()
        (self.source/'migrations/baseline.sql').write_text('SELECT 1;\n')
        (self.source/'internal/database').mkdir(parents=True)
        (self.source/'internal/database/database.go').write_text('package database\n')
        (self.source/'configs').mkdir()
        (self.source/'configs/configs.go').write_text('DBMaxOpenConns int `env:"DB_MAX_OPEN_CONNS" envDefault:"8"`\n')
        self.git('add','.')
        self.git('commit','-qm','synthetic migration baseline')
        self.baseline_commit=self.git('rev-parse','HEAD').stdout.strip()
        (self.source/'configs/configs.go').write_text('DBMaxOpenConns int `env:"DB_MAX_OPEN_CONNS" envDefault:"4"`\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'candidate pool four')
        (self.path/'az').write_text(FAKE_AZ)
        (self.path/'az').chmod(0o755)
        self.data = {'app': json.loads(json.dumps(APP)), 'revisions': {'bible-api--old': json.loads(json.dumps(BASE))},
            'gateway': {'properties': {'latestRevisionName':'api-gateway--http','latestReadyRevisionName':'api-gateway--http',
                'template':{'containers':[{'image':'alive.azurecr.io/alive/api-gateway@sha256:'+'c'*64}]}}},
            'jobs': [], 'executions': [], 'replicas': [{'properties': {'containers': [{'name': 'bible-api', 'ready': True}, {'name': 'daprd', 'ready': True}]}}]}
        self.env = dict(os.environ, PATH=str(self.path)+os.pathsep+os.environ['PATH'], CASE_DIR=str(self.path),
            RESOURCE_GROUP='fixture', API_APP_NAME='bible-api', API_GATEWAY_APP_NAME='api-gateway',
            GITHUB_ENV=str(self.path/'github-env'), RUNNER_TEMP=str(self.path), GITHUB_RUN_ID='123', GITHUB_RUN_ATTEMPT='1',
            IMAGE_REF=NEW_IMAGE, AZURE_SUBSCRIPTION_ID='11111111-1111-1111-1111-111111111111')
        self.review = {'baselineCommit': self.baseline_commit, 'baselineImage': IMAGE, 'baselinePoolMaxOpen': 8,
            'gatewayHttpVerified':True, 'gatewayHttpRevision':'api-gateway--http',
            'gatewayHttpImage':'alive.azurecr.io/alive/api-gateway@sha256:'+'c'*64,
            'importsFrozen': True,
            'verifiedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'evidence': 'fixture-reviewed-baseline'}

    def git(self,*args):
        return subprocess.run(['git','-c','user.name=Fixture','-c','user.email=fixture@example.invalid',
            '-c','core.hooksPath=/dev/null','-c','commit.gpgsign=false',*args],cwd=self.source,
            capture_output=True,text=True,check=True)

    def run_cli(self, command, ok=True):
        (self.path/'azure.json').write_text(json.dumps(self.data))
        self.env['BIBLE_RELEASE_REVIEW'] = json.dumps(self.review)
        proc = subprocess.run(['python3', str(ROOT/'scripts/release-runtime.py'), command], env=self.env,
            capture_output=True, text=True, timeout=10,cwd=self.source)
        if ok:
            self.assertEqual(proc.returncode, 0, proc.stdout+proc.stderr)
        else:
            self.assertNotEqual(proc.returncode, 0, proc.stdout+proc.stderr)
        self.data=json.loads((self.path/'azure.json').read_text())
        if (self.path/'github-env').exists():
            for line in (self.path/'github-env').read_text().splitlines():
                key,value=line.split('=',1);self.env[key]=value
        return proc

    def test_source_migration_change_blocks_release(self):
        (self.source/'migrations/baseline.sql').write_text('SELECT 2;\n')
        self.git('add','.')
        self.git('commit','-qm','incompatible migration change')
        self.run_cli('capture',False)
        self.assertFalse((self.path/'updated').exists())

    def test_pool_review_supports_eight_to_four_and_four_to_four(self):
        self.run_cli('capture')
        self.review['baselineCommit'] = self.git('rev-parse', 'HEAD').stdout.strip()
        self.review['baselinePoolMaxOpen'] = 4
        self.run_cli('capture')
        self.data['revisions']['bible-api--old']['properties']['template']['containers'][0]['env'] = [
            {'name': 'DB_MAX_OPEN_CONNS', 'value': '4'}]
        self.run_cli('capture')

    def test_pool_review_rejects_wrong_review_source_and_inherited_override(self):
        self.review['baselinePoolMaxOpen'] = 4
        self.run_cli('capture', False)
        self.review['baselinePoolMaxOpen'] = 8
        env = self.data['revisions']['bible-api--old']['properties']['template']['containers'][0]['env']
        for entry in [{'name':'DB_MAX_OPEN_CONNS','value':'8'},
                      {'name':'DB_MAX_OPEN_CONNS','secretRef':'pool'},
                      {'name':'DB_MAX_OPEN_CONNS','value':'4'}]:
            env[:] = [entry]
            self.run_cli('capture', False)
        env.clear()
        for default in (8, 12):
            (self.source/'configs/configs.go').write_text(
                f'DBMaxOpenConns int `env:"DB_MAX_OPEN_CONNS" envDefault:"{default}"`\n')
            self.git('add', '.')
            self.git('commit', '-qm', 'unreviewed candidate pool')
            self.run_cli('capture', False)
        self.assertFalse((self.path/'updated').exists())

    def reviewed_startup_change(self):
        (self.source/'internal/database/database.go').write_text('package database\n// reviewed factory\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'reviewed startup change')
        self.review['startupReview'] = {
            'baselineCommit': self.baseline_commit,
            'candidateCommit': self.git('rev-parse', 'HEAD').stdout.strip(),
            'baselineBlob': self.git('rev-parse', self.baseline_commit+':internal/database/database.go').stdout.strip(),
            'candidateBlob': self.git('rev-parse', 'HEAD:internal/database/database.go').stdout.strip(),
        }

    def test_exact_startup_review_allows_only_reviewed_change(self):
        self.reviewed_startup_change()
        self.run_cli('capture')
        approval = self.review.pop('startupReview')
        self.run_cli('capture', False)
        self.review['startupReview'] = approval
        for field in approval:
            with self.subTest(field=field):
                original = approval[field]
                approval[field] = '0' * 40
                self.run_cli('capture', False)
                approval[field] = original
        self.review['verifiedAt'] = '2020-01-01T00:00:00+00:00'
        self.run_cli('capture', False)
        self.assertFalse((self.path/'updated').exists())

    def test_startup_review_cannot_authorize_later_commit_or_migration(self):
        self.reviewed_startup_change()
        self.git('commit', '--allow-empty', '-qm', 'unreviewed candidate')
        self.run_cli('capture', False)
        self.review['startupReview']['candidateCommit'] = self.git('rev-parse', 'HEAD').stdout.strip()
        (self.source/'migrations/baseline.sql').write_text('SELECT 2;\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'unreviewed migration')
        self.review['startupReview']['candidateCommit'] = self.git('rev-parse', 'HEAD').stdout.strip()
        self.run_cli('capture', False)
        self.assertFalse((self.path/'updated').exists())

    def test_zero_activation_requires_current_gateway_http_evidence(self):
        self.run_cli('capture')
        for field, value in [('gatewayHttpVerified',False), ('gatewayHttpRevision','api-gateway--old'),
                             ('gatewayHttpImage',IMAGE)]:
            with self.subTest(field=field):
                old=self.review[field]; self.review[field]=value
                self.run_cli('update',False)
                self.assertFalse((self.path/'updated').exists())
                self.review[field]=old

    def test_zero_activation_cannot_skip_private_http_preparation(self):
        for missing in ('ingress','http-scaler'):
            with self.subTest(missing=missing):
                self.data['app']['properties']['configuration']['ingress']=None if missing=='ingress' else RUNTIME['ingress']
                rules=self.data['revisions']['bible-api--old']['properties']['template']['scale']['rules']
                original=json.loads(json.dumps(rules))
                if missing=='http-scaler': rules[:]=[rule for rule in rules if 'http' not in rule]
                self.run_cli('capture')
                self.run_cli('update',False)
                self.assertFalse((self.path/'updated').exists())
                rules[:]=original

    def test_capture_wakes_zero_scaled_baseline_and_refuses_supersession(self):
        spec=importlib.util.spec_from_file_location('runtime_zero',ROOT/'scripts/release-runtime.py')
        runtime=importlib.util.module_from_spec(spec);spec.loader.exec_module(runtime)
        initial=json.loads(json.dumps(APP['properties']))
        initial['configuration']['ingress']=RUNTIME['ingress']
        cold=json.loads(json.dumps(BASE['properties']))
        cold['template']['scale']=RUNTIME['scale']
        cold['runningState']='ScaledToZero'
        ready=json.loads(json.dumps(cold));ready['runningState']='Running'
        for superseded in (False,True):
            current=json.loads(json.dumps(initial))
            if superseded: current['latestRevisionName']='bible-api--foreign'
            response=mock.MagicMock()
            response.__enter__.return_value.status=200
            response.__enter__.return_value.read.return_value=b'[]'
            with mock.patch.dict(os.environ,self.env), mock.patch.object(runtime,'app',side_effect=[initial,current]), \
                    mock.patch.object(runtime,'revision',side_effect=[cold,ready]), \
                    mock.patch.object(runtime,'urlopen',return_value=response) as wake, \
                    mock.patch.object(runtime,'immutable',return_value=IMAGE), \
                    mock.patch.object(runtime,'check_release_boundaries',return_value={'bible-api--old'}):
                if superseded:
                    with self.assertRaisesRegex(ValueError,'superseded'): runtime.capture()
                else:
                    runtime.capture()
                    wake.assert_called_once_with('https://www.alive.org.tw/api/bible/v2/versions',timeout=120)

    def test_update_reactivates_baseline_that_scaled_down_during_image_build(self):
        state=self.data['revisions']['bible-api--old']['properties']
        state['template']['scale']=json.loads(json.dumps(RUNTIME['scale']))
        self.run_cli('capture')
        state=self.data['revisions']['bible-api--old']['properties']
        state['runningState']='ScaleTo0'
        self.data['updated_app']={'properties':{'latestRevisionName':'bible-api--release-123-1'}}
        (self.path/'azure.json').write_text(json.dumps(self.data))
        spec=importlib.util.spec_from_file_location('runtime_reactivate',ROOT/'scripts/release-runtime.py')
        runtime=importlib.util.module_from_spec(spec);spec.loader.exec_module(runtime)
        def activated(_limit):
            state['runningState']='Running'
            (self.path/'azure.json').write_text(json.dumps(self.data))
            return b'[]'
        response=mock.MagicMock()
        response.__enter__.return_value.status=200
        response.__enter__.return_value.read.side_effect=activated
        with mock.patch.dict(os.environ,self.env), mock.patch.object(runtime,'urlopen',return_value=response) as wake, \
                contextlib.chdir(self.source):
            runtime.update()
            wake.assert_called_once_with('https://www.alive.org.tw/api/bible/v2/versions',timeout=120)
        self.assertTrue((self.path/'updated').exists())

    def test_capture_preserves_original_probe_and_template(self):
        self.run_cli('capture')
        self.assertEqual(self.env['PREVIOUS_READY_REVISION'], 'bible-api--old')
        self.assertEqual(self.env['PREVIOUS_IMAGE_REF'], IMAGE)
        self.assertEqual(self.env['PREVIOUS_READINESS_PATH'], '/health')
        self.assertFalse((self.path/'updated').exists())

    def test_capture_rejects_stale_unhealthy_and_unreviewed_baselines(self):
        for field,value in [('active',False), ('healthState','Unhealthy'), ('runningState','Stopped')]:
            with self.subTest(field=field):
                self.data['revisions']['bible-api--old']['properties'][field]=value
                self.run_cli('capture',False)
                self.data['revisions']['bible-api--old']['properties'][field]=BASE['properties'][field]
        for field,value in [('baselineCommit','bad'), ('baselineImage',NEW_IMAGE), ('baselinePoolMaxOpen',0),
                ('importsFrozen',False), ('verifiedAt','2020-01-01T00:00:00+00:00')]:
            with self.subTest(field=field):
                old=self.review[field];self.review[field]=value
                self.run_cli('capture',False);self.review[field]=old
        self.data['jobs']=[{'name':'bible-import'}]
        self.data['executions']=[{'properties': {'status':'Running'}}]
        self.run_cli('capture',False)
        self.assertFalse((self.path/'updated').exists())

    def test_capture_rejects_unreviewed_request_scaling(self):
        for field, value in [('minReplicas', 2), ('maxReplicas', 3), ('rules', [])]:
            with self.subTest(field=field):
                scale = self.data['revisions']['bible-api--old']['properties']['template']['scale']
                old = scale[field]; scale[field] = value
                self.run_cli('capture', False)
                scale[field] = old
        self.assertFalse((self.path/'updated').exists())

    def test_capture_resolves_registry_tag_to_digest(self):
        self.data['revisions']['bible-api--old']['properties']['template']['containers'][0]['image']='alive.azurecr.io/alive/bible-api:main-5c2b666'
        self.data['registry']={'digest':'sha256:'+'a'*64}
        self.run_cli('capture')
        self.assertEqual(self.env['PREVIOUS_IMAGE_REF'],IMAGE)

    def test_wait_does_not_accept_old_ready_or_unready_sidecar(self):
        self.run_cli('capture')
        name='bible-api--release-123-1'
        self.candidate(name,NEW_IMAGE)
        self.env['CANDIDATE_REVISION']=name
        sys.dont_write_bytecode=True
        spec=importlib.util.spec_from_file_location('bible_release_runtime',ROOT/'scripts/release-runtime.py')
        runtime=importlib.util.module_from_spec(spec);spec.loader.exec_module(runtime)
        for old_ready in (True,False):
            with self.subTest(old_ready=old_ready):
                self.data['app']['properties']['latestReadyRevisionName']='bible-api--old' if old_ready else name
                self.data['replicas'][0]['properties']['containers'][1]['ready']=False
                (self.path/'azure.json').write_text(json.dumps(self.data))
                with mock.patch.dict(os.environ,self.env), mock.patch.object(runtime.time,'monotonic',side_effect=[0,1,601]), mock.patch.object(runtime.time,'sleep'):
                    with self.assertRaisesRegex(ValueError,'deadline'):
                        runtime.wait(name,NEW_IMAGE)

    def test_capture_rejects_pool_override(self):
        container = self.data['revisions']['bible-api--old']['properties']['template']['containers'][0]
        original = container.get('env', [])
        for entry in [{'name': 'DB_MAX_OPEN_CONNS', 'value': '9'},
                      {'name': 'DB_MAX_OPEN_CONNS', 'secretRef': 'unknown-pool'}]:
            with self.subTest(entry=entry):
                self.data['revisions']['bible-api--old']['properties']['template']['containers'][0]['env'] = [entry]
                self.run_cli('capture', False)
        self.data['revisions']['bible-api--old']['properties']['template']['containers'][0]['env'] = original
        self.run_cli('capture')
        self.assertFalse((self.path/'updated').exists())

    def candidate(self, name, image):
        revision=json.loads(json.dumps(BASE));revision['name']=name
        revision['properties']['template']['containers'][0]['image']=image
        revision['properties']['template']['containers'][0]['probes'][0]['httpGet']['path']='/ready'
        revision['properties']['template']['containers'][0]['env'].append({'name':'BIBLE_GATEWAY_TOKEN','secretRef':'bible-gateway-token'})
        revision['properties']['template']['scale']=json.loads(json.dumps(RUNTIME['scale']))
        self.data['revisions'][name]=revision
        self.data['app']['properties'].update(latestRevisionName=name,latestReadyRevisionName=name)
        return revision

    def test_update_pins_candidate_then_waits_for_exact_identity(self):
        self.run_cli('capture')
        name='bible-api--release-123-1'
        self.data['updated_app']={'properties':{'latestRevisionName':name}}
        self.run_cli('update')
        self.assertEqual(self.env['CANDIDATE_REVISION'],name)
        calls = [json.loads(line) for line in (self.path/'calls').read_text().splitlines()]
        updates = [args for args in calls if args[0] == 'rest']
        self.assertEqual(len(updates), 1)
        self.assertEqual(updates[0][updates[0].index('--method')+1], 'PATCH')
        self.assertEqual(updates[0][updates[0].index('--body')+1], '@/dev/stdin')
        self.assertEqual(updates[0][updates[0].index('--url')+1],
            'https://management.azure.com/subscriptions/11111111-1111-1111-1111-111111111111/resourceGroups/fixture/providers/Microsoft.App/containerApps/bible-api?api-version=2026-01-01')
        expected = json.loads(json.dumps(TEMPLATE))
        expected['containers'][0]['image'] = NEW_IMAGE
        expected['containers'][0]['probes'][0]['httpGet']['path'] = '/ready'
        expected['containers'][0]['env'].append({'name':'BIBLE_GATEWAY_TOKEN','secretRef':'bible-gateway-token'})
        expected['scale'] = RUNTIME['scale']
        expected['revisionSuffix'] = 'release-123-1'
        self.assertEqual(json.loads((self.path/'patch.json').read_text()), {'properties': {'template': expected,
            'configuration': {'ingress':RUNTIME['ingress'],'dapr':APP['properties']['configuration']['dapr']}}})
        self.candidate(name,NEW_IMAGE)
        self.run_cli('wait')
        self.data['app']['properties']['latestRevisionName']='bible-api--unrelated'
        self.run_cli('wait',False)
        self.data['app']['properties']['latestRevisionName']=name
        self.data['revisions'][name]['properties']['template']['containers'][0]['image']=IMAGE
        self.run_cli('wait',False)
        self.data['app']='DENIED'
        self.run_cli('wait',False)

    def test_ingress_is_enabled_only_after_authenticated_image_is_ready(self):
        self.prepare_updated_candidate()
        self.data['app']['properties']['latestReadyRevisionName']='bible-api--old'
        self.run_cli('configure',False)
        self.assertFalse((self.path/'configured').exists())
        self.data['app']['properties']['latestReadyRevisionName']='bible-api--release-123-1'
        self.run_cli('configure')
        self.assertFalse(self.data['app']['properties']['configuration']['ingress']['external'])
        self.assertEqual(self.data['app']['properties']['configuration']['ingress']['targetPort'],9999)
        self.run_cli('wait')
        self.run_cli('rollback')
        self.assertEqual(self.data['app']['properties']['configuration']['ingress'],RUNTIME['ingress'])
        self.assertTrue(self.data['app']['properties']['configuration']['dapr']['enabled'])

    def test_configure_waits_for_owned_async_app_scope_readback(self):
        self.prepare_updated_candidate()
        self.data['configure_pending_reads']=2
        (self.path/'azure.json').write_text(json.dumps(self.data))
        spec=importlib.util.spec_from_file_location('runtime_async',ROOT/'scripts/release-runtime.py')
        runtime=importlib.util.module_from_spec(spec);spec.loader.exec_module(runtime)
        with mock.patch.dict(os.environ,self.env), mock.patch.object(runtime.time,'sleep') as pause:
            runtime.configure()
        actual=json.loads((self.path/'azure.json').read_text())['app']['properties']['configuration']
        self.assertEqual(actual['ingress'],RUNTIME['ingress'])
        # subprocess polling can also call time.sleep; count only transport waits.
        self.assertEqual(pause.call_args_list.count(mock.call(2)),2)

    def test_rollback_accepts_async_response_then_verifies_exact_revision(self):
        self.prepare_updated_candidate()
        self.data['copied_app']={}
        self.run_cli('rollback')
        self.assertEqual(self.env['ROLLBACK_REVISION'],'bible-api--rollback-123-1')

    def test_configure_refuses_supersession_and_has_a_deadline(self):
        self.prepare_updated_candidate()
        (self.path/'azure.json').write_text(json.dumps(self.data))
        spec=importlib.util.spec_from_file_location('runtime_async_bounds',ROOT/'scripts/release-runtime.py')
        runtime=importlib.util.module_from_spec(spec);spec.loader.exec_module(runtime)
        initial=json.loads(json.dumps(self.data['app']['properties']))
        foreign=json.loads(json.dumps(initial));foreign['latestRevisionName']='bible-api--foreign'
        foreign['configuration']['ingress']=RUNTIME['ingress']
        with mock.patch.dict(os.environ,self.env), mock.patch.object(runtime,'app',side_effect=[initial,foreign]):
            with self.assertRaisesRegex(ValueError,'superseded'): runtime.configure()
        self.data['configure_pending_reads']=99
        (self.path/'azure.json').write_text(json.dumps(self.data))
        with mock.patch.dict(os.environ,self.env), mock.patch.object(runtime.time,'monotonic',side_effect=[0,1,181]), \
                mock.patch.object(runtime.time,'sleep'):
            with self.assertRaisesRegex(ValueError,'deadline'): runtime.configure()

    def test_public_ingress_and_missing_credential_block_before_mutation(self):
        self.data['app']['properties']['configuration']['ingress']={'external':True,'targetPort':9999,'allowInsecure':False}
        self.run_cli('capture',False)
        self.data['app']['properties']['configuration'].pop('ingress')
        self.run_cli('capture')
        self.data['app']['properties']['configuration']['secrets']=[]
        self.run_cli('update',False)
        self.assertFalse((self.path/'updated').exists())

    def prepare_updated_candidate(self):
        self.run_cli('capture')
        name='bible-api--release-123-1'
        self.data['updated_app']={'properties':{'latestRevisionName':name}}
        self.run_cli('update')
        self.candidate(name,NEW_IMAGE)
        self.data['copied_app']={'properties':{'latestRevisionName':'bible-api--rollback-123-1'}}

    def test_ready_baseline_remains_ready_after_repeat_release_and_rollback(self):
        self.data['revisions']['bible-api--old']['properties']['template']['containers'][0]['probes'][0]['httpGet']['path'] = '/ready'
        self.prepare_updated_candidate()
        self.run_cli('wait')
        self.run_cli('rollback')
        self.assertEqual(self.data['revisions']['bible-api--rollback-123-1']['properties']['template']['containers'][0]['probes'][0]['httpGet']['path'], '/ready')

    def test_other_candidate_template_changes_block_forward_and_rollback(self):
        self.prepare_updated_candidate()
        template = self.data['revisions']['bible-api--release-123-1']['properties']['template']
        template['containers'][0]['probes'][1]['httpGet']['path'] = '/ready'
        self.run_cli('wait', False)
        self.run_cli('rollback', False)
        self.assertFalse((self.path/'copied').exists())

    def test_accepted_patch_waits_for_own_revision_without_accepting_baseline(self):
        self.run_cli('capture')
        self.data['updated_app'] = {}
        self.run_cli('update')
        spec=importlib.util.spec_from_file_location('runtime', ROOT/'scripts/release-runtime.py')
        module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
        candidate=json.loads(json.dumps(BASE['properties']))
        candidate['template']['containers'][0]['image']=NEW_IMAGE
        candidate['template']['containers'][0]['probes'][0]['httpGet']['path']='/ready'
        current=json.loads(json.dumps(APP['properties']))
        active=json.loads(json.dumps(current));active.update(latestRevisionName='bible-api--release-123-1',latestReadyRevisionName='bible-api--release-123-1')
        with mock.patch.dict(os.environ, self.env), mock.patch.object(module,'app',side_effect=[current,active]), mock.patch.object(module,'revision',return_value=candidate), mock.patch.object(module,'az',return_value=self.data['replicas']), mock.patch.object(module.time,'sleep'):
            module.wait('bible-api--release-123-1', NEW_IMAGE, module.template_hash(candidate['template']), previous='bible-api--old')

    def test_invalid_patch_target_stops_before_mutation_ownership(self):
        self.run_cli('capture')
        for key in ('AZURE_SUBSCRIPTION_ID', 'RESOURCE_GROUP', 'API_APP_NAME'):
            original = self.env[key]
            self.env[key] = 'foreign/path'
            self.run_cli('update', False)
            self.assertNotIn('RUNTIME_UPDATE_STARTED', self.env)
            self.assertFalse((self.path/'updated').exists())
            self.env[key] = original

    def test_inactive_zero_revisions_do_not_hold_database_pools(self):
        orphan=json.loads(json.dumps(BASE));orphan['name']='bible-api--zero'
        orphan['properties'].update(active=False,replicas=0)
        for state in ('ScaleTo0','ScaledToZero'):
            with self.subTest(state=state):
                orphan['properties']['runningState']=state
                self.data['revisions'][orphan['name']]=json.loads(json.dumps(orphan))
                self.run_cli('capture')

    def test_inactive_terminating_revision_blocks_capture(self):
        orphan=json.loads(json.dumps(BASE));orphan['name']='bible-api--terminating'
        orphan['properties'].update(active=False,replicas=1)
        self.data['revisions'][orphan['name']]=orphan
        self.run_cli('capture',False)

    def test_preflight_rejection_does_not_copy_even_if_failure_handler_runs(self):
        self.run_cli('capture')
        self.data['jobs']=[{'name':'bible-import'}]
        self.data['executions']=[{'properties':{'status':'Running'}}]
        self.run_cli('update',False)
        self.run_cli('rollback',False)
        self.assertFalse((self.path/'updated').exists())
        self.assertFalse((self.path/'copied').exists())

    def test_rollback_refuses_superseding_run(self):
        self.prepare_updated_candidate()
        self.data['app']['properties']['latestRevisionName']='bible-api--another-run'
        self.run_cli('rollback',False)
        self.assertFalse((self.path/'copied').exists())

    def test_rollback_refuses_import_started_after_update(self):
        self.prepare_updated_candidate()
        self.data['jobs']=[{'name':'bible-import'}]
        self.data['executions']=[{'properties':{'status':'Running'}}]
        self.run_cli('rollback',False)
        self.assertFalse((self.path/'copied').exists())

    def test_rollback_copies_original_template_and_checks_result(self):
        self.prepare_updated_candidate()
        self.run_cli('rollback')
        self.assertEqual(self.env['ROLLBACK_REVISION'],'bible-api--rollback-123-1')
        calls=[json.loads(x) for x in (self.path/'calls').read_text().splitlines()]
        patches=[x for x in calls if x[0]=='rest']
        self.assertEqual(len(patches),2)
        restored=json.loads((self.path/'patch.json').read_text())['properties']
        self.assertEqual(restored['template']['containers'][0]['image'],IMAGE)
        self.assertEqual(restored['configuration']['ingress'],RUNTIME['ingress'])
        self.assertTrue(restored['configuration']['dapr']['enabled'])

    def test_rollback_rejects_corrupted_copy_template(self):
        self.prepare_updated_candidate()
        self.data['corrupt_copy_probe']=True
        self.run_cli('rollback',False)

if __name__=='__main__':unittest.main()
