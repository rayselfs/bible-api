#!/usr/bin/env python3
"""Exercise the real release CLI against a strict fake Azure boundary."""
import datetime
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
IMAGE = 'alive.azurecr.io/alive/bible-api@sha256:' + 'a' * 64
NEW_IMAGE = 'alive.azurecr.io/alive/bible-api@sha256:' + 'b' * 64
TEMPLATE = {'containers': [{'name': 'bible-api', 'image': IMAGE, 'probes': [
    {'type': 'Readiness', 'httpGet': {'path': '/health', 'port': 9999}},
    {'type': 'Liveness', 'httpGet': {'path': '/health', 'port': 9999}}], 'env': []}],
    'scale': {'minReplicas': 1, 'maxReplicas': 8, 'rules': [{'name': 'cpu-scaler', 'custom': {'type': 'cpu', 'metadata': {'type': 'Utilization', 'value': '70'}}}]}}
BASE = {'name': 'bible-api--old', 'properties': {'active': True, 'healthState': 'Healthy',
    'runningState': 'RunningAtMaxScale', 'template': TEMPLATE}}
APP = {'properties': {'latestReadyRevisionName': 'bible-api--old', 'latestRevisionName': 'bible-api--old',
    'configuration': {'activeRevisionsMode': 'Single', 'dapr': {'enabled': True, 'appId': 'bible-api', 'appPort': 9999}}}}

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
elif args[:2]==['containerapp','show']:result=data['app']
elif args[:2]==['containerapp','update']:
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
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.source = self.path/'source'
        self.source.mkdir()
        self.git('init','-q')
        (self.source/'migrations').mkdir()
        (self.source/'migrations/baseline.sql').write_text('SELECT 1;\n')
        self.git('add','.')
        self.git('commit','-qm','synthetic migration baseline')
        self.baseline_commit=self.git('rev-parse','HEAD').stdout.strip()
        (self.path/'az').write_text(FAKE_AZ)
        (self.path/'az').chmod(0o755)
        self.data = {'app': json.loads(json.dumps(APP)), 'revisions': {'bible-api--old': json.loads(json.dumps(BASE))},
            'jobs': [], 'executions': [], 'replicas': [{'properties': {'containers': [{'name': 'bible-api', 'ready': True}, {'name': 'daprd', 'ready': True}]}}]}
        self.env = dict(os.environ, PATH=str(self.path)+os.pathsep+os.environ['PATH'], CASE_DIR=str(self.path),
            RESOURCE_GROUP='fixture', API_APP_NAME='bible-api', DAPR_APP_ID='bible-api',
            GITHUB_ENV=str(self.path/'github-env'), RUNNER_TEMP=str(self.path), GITHUB_RUN_ID='123', GITHUB_RUN_ATTEMPT='1',
            IMAGE_REF=NEW_IMAGE)
        self.review = {'baselineCommit': self.baseline_commit, 'baselineImage': IMAGE, 'baselinePoolMaxOpen': 8, 'ordinarySlots': 844,
            'otherConnectionsCeiling': 552, 'importsFrozen': True,
            'verifiedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'evidence': 'fixture-reviewed-budget'}

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

    def test_capture_preserves_original_probe_and_template(self):
        self.run_cli('capture')
        self.assertEqual(self.env['PREVIOUS_READY_REVISION'], 'bible-api--old')
        self.assertEqual(self.env['PREVIOUS_IMAGE_REF'], IMAGE)
        self.assertEqual(self.env['PREVIOUS_READINESS_PATH'], '/health')
        self.assertFalse((self.path/'updated').exists())

    def test_capture_rejects_stale_unhealthy_and_unbudgeted_baselines(self):
        for field,value in [('active',False), ('healthState','Unhealthy'), ('runningState','Stopped')]:
            with self.subTest(field=field):
                self.data['revisions']['bible-api--old']['properties'][field]=value
                self.run_cli('capture',False)
                self.data['revisions']['bible-api--old']['properties'][field]=BASE['properties'][field]
        for field,value in [('baselineCommit','bad'), ('baselineImage',NEW_IMAGE), ('baselinePoolMaxOpen',0), ('otherConnectionsCeiling',553),
                ('importsFrozen',False), ('verifiedAt','2020-01-01T00:00:00+00:00')]:
            with self.subTest(field=field):
                old=self.review[field];self.review[field]=value
                self.run_cli('capture',False);self.review[field]=old
        self.data['jobs']=[{'name':'bible-import'}]
        self.data['executions']=[{'properties': {'status':'Running'}}]
        self.run_cli('capture',False)
        self.assertFalse((self.path/'updated').exists())

    def test_capture_rejects_unreviewed_request_scaling(self):
        for field, value in [('minReplicas', 0), ('maxReplicas', 3), ('rules', [])]:
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

    def test_platform_reserve_must_be_explicitly_reviewed(self):
        self.data['revisions']['bible-api--old']['properties']['template']['scale']['maxReplicas']=8
        self.review['otherConnectionsCeiling']=592
        self.run_cli('capture',False)
        self.review['reserveConnections']=31
        self.run_cli('capture',False)
        self.review['reserveConnections']=32
        self.run_cli('capture')

    def candidate(self, name, image):
        revision=json.loads(json.dumps(BASE));revision['name']=name
        revision['properties']['template']['containers'][0]['image']=image
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
        updates = [args for args in calls if args[:2] == ['containerapp', 'update']]
        self.assertEqual(updates, [['containerapp', 'update', '-g', 'fixture', '-n', 'bible-api',
            '--image', NEW_IMAGE, '--revision-suffix', 'release-123-1', '--only-show-errors', '--output', 'json']])
        self.candidate(name,NEW_IMAGE)
        self.run_cli('wait')
        self.data['app']['properties']['latestRevisionName']='bible-api--unrelated'
        self.run_cli('wait',False)
        self.data['app']['properties']['latestRevisionName']=name
        self.data['revisions'][name]['properties']['template']['containers'][0]['image']=IMAGE
        self.run_cli('wait',False)
        self.data['app']='DENIED'
        self.run_cli('wait',False)

    def prepare_updated_candidate(self):
        self.run_cli('capture')
        name='bible-api--release-123-1'
        self.data['updated_app']={'properties':{'latestRevisionName':name}}
        self.run_cli('update')
        self.candidate(name,NEW_IMAGE)
        self.data['copied_app']={'properties':{'latestRevisionName':'bible-api--rollback-123-1'}}

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
        copy=next(x for x in calls if x[:3]==['containerapp','revision','copy'])
        self.assertEqual(copy[copy.index('--from-revision')+1],'bible-api--old')
        self.assertEqual(copy[copy.index('--image')+1],IMAGE)

    def test_rollback_rejects_corrupted_copy_template(self):
        self.prepare_updated_candidate()
        self.data['corrupt_copy_probe']=True
        self.run_cli('rollback',False)

if __name__=='__main__':unittest.main()
