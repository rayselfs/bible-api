import copy
import importlib.util
import json
import subprocess
import sys
from pathlib import Path
import unittest
import tempfile
from unittest.mock import patch

ROOT = Path(__file__).parent
spec = importlib.util.spec_from_file_location('isolation', ROOT / 'isolation.py')


class IsolationTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue((ROOT / 'isolation.py').exists(), 'missing isolation lifecycle implementation')
        self.m = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.m)
        self.env = dict(GITHUB_REF='refs/heads/main', GITHUB_REPOSITORY='rayselfs/bible-api',
                        GITHUB_RUN_ID='123', GITHUB_RUN_ATTEMPT='1', GITHUB_SHA='a' * 40,
                        AZURE_SUBSCRIPTION_ID='3765a72c-2c30-492c-8b88-7a26522903f8')
        self.ctx = self.m.Context(self.env)
        self.image = 'alive.azurecr.io/alive/bible-api@sha256:' + 'b' * 64
        self.helper = 'alive.azurecr.io/alive/bible-readiness-fixture@sha256:' + 'c' * 64

    def test_pg_query_survives_console_word_splitting(self):
        sql = "SELECT count(*) FROM pg_stat_activity WHERE application_name = 'old';"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            executable = root / 'psql'
            executable.write_text('#!/usr/bin/env python3\nimport sys\nfrom pathlib import Path\n'
                                  + f'Path({str(root / "sql")!r}).write_text(sys.argv[-1])\nprint("0")\n')
            executable.chmod(0o755)
            import os
            env = dict(os.environ, PATH=directory + os.pathsep + os.environ['PATH'])
            class Console:
                def console(self, app, revision, replica, command):
                    result = subprocess.run(command.split(), env=env, capture_output=True, text=True)
                    return result.stdout
            self.assertEqual(self.m.pg_sql(self.ctx, Console(), 'revision', 'replica', sql), 0)
            self.assertEqual((root / 'sql').read_text(), sql)

    def test_context_rejects_untrusted_ref_repository_and_noncanonical_ids(self):
        for key, bad in [('GITHUB_REF', 'refs/heads/feature'), ('GITHUB_REPOSITORY', 'fork/bible-api'),
                         ('GITHUB_RUN_ID', '../alive'), ('GITHUB_RUN_ID', '01'),
                         ('GITHUB_RUN_ATTEMPT', '0'), ('AZURE_SUBSCRIPTION_ID', 'other')]:
            with self.subTest(key=key, bad=bad), self.assertRaises(ValueError):
                self.m.Context(dict(self.env, **{key: bad}))
        self.assertEqual(self.ctx.group, 'hhc-bible-ready-123-1')

    def test_manifest_isolates_identity_database_dapr_and_ingress(self):
        plan = self.m.plan(self.ctx, self.image, self.helper)
        # ARM requires null for disabled logs; the CLI's "none" is not a wire value.
        self.assertIsNone(plan['environment']['properties']['appLogsConfiguration']['destination'])
        self.assertEqual(plan['target']['properties']['configuration']['activeRevisionsMode'], 'Single')
        self.assertNotIn('ingress', plan['target']['properties']['configuration'])
        self.assertEqual(plan['target']['properties']['configuration']['dapr']['appId'], 'bible-ready-target')
        self.assertEqual(plan['target']['properties']['template']['scale']['maxReplicas'], 3)
        self.assertEqual(plan['pg']['properties']['template']['scale']['maxReplicas'], 1)
        text = str(plan)
        for forbidden in ('hhc-pg', 'alive-vault', 'alive-env', 'keyVaultUrl', 'www.alive.org.tw'):
            self.assertNotIn(forbidden, text)
        self.assertEqual(plan['role']['principalId'], 'UNRESOLVED_NEW_ENVIRONMENT_IDENTITY')
        with self.assertRaises(ValueError):
            self.m.plan(self.ctx, self.image.replace('sha256:', 'tag:'), self.helper)

    def test_cleanup_never_deletes_foreign_group_or_role(self):
        cloud = FakeCloud(self.ctx)
        cloud.group['tags']['hhc-run'] = '999-1'
        with self.assertRaises(ValueError):
            self.m.cleanup(self.ctx, cloud)
        self.assertEqual(cloud.deleted, [])
        cloud = FakeCloud(self.ctx)
        cloud.role['properties']['principalId'] = '22222222-2222-2222-2222-222222222222'
        with self.assertRaises(ValueError):
            self.m.cleanup(self.ctx, cloud)
        self.assertEqual(cloud.deleted, [])

    def test_cleanup_deletes_only_exact_owned_role_then_group(self):
        cloud = FakeCloud(self.ctx)
        self.m.cleanup(self.ctx, cloud)
        self.assertEqual(cloud.deleted, [self.ctx.role_id, self.ctx.group_id])

    def test_role_grant_requires_exact_external_review_marker(self):
        cloud = FakeCloud(self.ctx)
        cloud.role = None
        review = self.m.role_plan(self.ctx, cloud.environment)
        with self.assertRaises(ValueError):
            self.m.grant_reviewed_role(self.ctx, cloud, review)
        self.assertEqual(cloud.written, [])
        cloud.group['tags']['hhc-role-review'] = self.m.digest(review)
        self.m.grant_reviewed_role(self.ctx, cloud, review)
        self.assertEqual(cloud.written, [self.ctx.role_id])
        self.assertEqual(cloud.role['properties']['principalId'], '11111111-1111-1111-1111-111111111111')

    def test_assessment_demands_complete_nonce_revision_cadence_and_true_fault(self):
        samples = [dict(phase='fault', mono=i, nonce=f'iso-123-1-{i}', status=200,
                        valid=True, version=1+i%2) for i in range(602)]
        routes = {s['nonce']: {'old'} for s in samples}
        phases = {'fault': dict(seconds=600, revision='old')}
        self.assertEqual(self.m.assess(samples, routes, phases, True), 'PASS')
        self.assertEqual(self.m.assess(samples, routes, phases, False), 'UNKNOWN')
        self.assertEqual(self.m.assess(samples[:50], routes, phases, True), 'UNKNOWN')
        lost = dict(routes)
        lost.pop(samples[5]['nonce'])
        self.assertEqual(self.m.assess(samples, lost, phases, True), 'UNKNOWN')
        wrong = copy.deepcopy(routes)
        wrong[samples[5]['nonce']] = {'candidate'}
        self.assertEqual(self.m.assess(samples, wrong, phases, True), 'FAIL')
        gap = [s for s in samples if not 20 <= s['mono'] <= 25]
        self.assertEqual(self.m.assess(gap, routes, phases, True), 'UNKNOWN')
        broken = copy.deepcopy(samples)
        broken[2]['status'] = 500
        self.assertEqual(self.m.assess(broken, routes, phases, True), 'FAIL')
        broken[2]['status'], broken[2]['valid'] = 200, False
        self.assertEqual(self.m.assess(broken, routes, phases, True), 'FAIL')

    def test_console_management_errors_or_duplicate_frames_are_unknown(self):
        import base64
        frame = 'ISOLATION_abc=' + base64.b64encode(b'{"armed":true}').decode()
        self.assertEqual(self.m.console_result(0, frame, 'abc'), {'armed': True})
        for code, text in [(1, frame), (0, '403 denied'), (0, frame+'\n'+frame)]:
            with self.subTest(code=code, text=text), self.assertRaises(RuntimeError):
                self.m.console_result(code, text, 'abc')

    def test_cloud_refuses_any_unreviewed_write_target_before_cli(self):
        cloud = self.m.Cloud(self.ctx)
        for resource in [self.ctx.registry_id, self.ctx.group_id.replace(self.ctx.group, 'alive'),
                         self.ctx.group_id+'/providers/Microsoft.App/containerApps/api-gateway']:
            with self.subTest(resource=resource), self.assertRaises(ValueError):
                cloud.put(resource, {})

    def test_console_uses_platform_script_syntax_without_changing_target(self):
        cloud = self.m.Cloud(self.ctx)
        for platform in ('darwin', 'linux'):
            with self.subTest(platform=platform), patch.object(self.m.sys, 'platform', platform), \
                    patch.object(self.m, 'bounded', return_value='frame') as run:
                self.assertEqual(cloud.console('bible-ready-caller', 'bible-ready-caller--candidate',
                                              'bible-ready-caller--candidate-replica', '/fixture client state abc'),
                                 'frame')
                args, timeout = run.call_args.args
                self.assertEqual(timeout, 90)
                if platform == 'darwin':
                    self.assertEqual(args[:4], ['script', '-q', '/dev/null', 'az'])
                    self.assertEqual(args[-1], '/fixture client state abc')
                else:
                    self.assertEqual(args[:4], ['script', '-q', '-e', '-c'])
                    self.assertIn('bible-ready-caller--candidate-replica', args[4])

    def test_preview_is_pure_and_never_claims_resolved_principal(self):
        import tempfile
        with tempfile.TemporaryDirectory() as folder:
            env = dict(__import__('os').environ, **self.env, BIBLE_IMAGE_REF=self.image, HELPER_IMAGE_REF=self.helper,
                       ISOLATION_OUTPUT=folder, PATH='')
            proc = subprocess.run([sys.executable, str(ROOT/'isolation.py'), 'preview'], env=env,
                                  capture_output=True,text=True,timeout=5)
            self.assertEqual(proc.returncode,0,proc.stderr)
            preview=json.loads((Path(folder)/'preview.json').read_text())
            self.assertEqual(preview['role']['principalId'],'UNRESOLVED_NEW_ENVIRONMENT_IDENTITY')

    def test_fault_evidence_requires_held_port_and_app_healthy_for_whole_window(self):
        observations=[dict(phase='fault',mono=i,held=True,health=200,ready=503,outbound=0) for i in range(602)]
        self.assertTrue(self.m.fault_window(observations,'fault',600))
        observations[3]['health']=0
        self.assertFalse(self.m.fault_window(observations,'fault',600))
        observations[3]['health']=200
        observations[4]['ready']=200
        self.assertFalse(self.m.fault_window(observations,'fault',600))

    def test_log_association_uses_nonce_and_exact_captured_revision(self):
        import tempfile
        with tempfile.TemporaryDirectory() as folder:
            path=Path(folder)/'old-bible.log'
            path.write_text(json.dumps({'Log':'GET /api/bible/v1/versions?isolation_nonce=iso-123-1-5'})+'\n')
            self.assertEqual(self.m.nonce_routes([(path,'old')]), {'iso-123-1-5':{'old'}})
            path.write_text('not JSON\n')
            with self.assertRaises(RuntimeError):
                self.m.nonce_routes([(path,'old')])

    def test_release_provenance_rejects_failed_nonmain_or_other_workflow(self):
        release=dict(path='.github/workflows/release.yml',head_branch='main',head_sha='d'*40,
                     status='completed',conclusion='success',event='push')
        self.assertEqual(self.m.release_commit(release),'d'*40)
        for key,value in [('conclusion','failure'),('head_branch','feature'),('path','.github/workflows/ci.yml'),('event','pull_request')]:
            with self.subTest(key=key),self.assertRaises(ValueError):
                self.m.release_commit(dict(release,**{key:value}))

    def test_immutable_release_receipt_must_match_exact_run_and_commit(self):
        pointer=dict(commit='d'*40,image=self.image,releaseUrl='https://github.com/rayselfs/bible-api/actions/runs/222')
        self.assertEqual(self.m.receipt_image(pointer,'222','d'*40),self.image)
        for key,value in [('commit','e'*40),('releaseUrl','https://github.com/rayselfs/bible-api/actions/runs/223'),
                          ('image','alive.azurecr.io/alive/bible-api:main-ddddddd')]:
            with self.subTest(key=key),self.assertRaises(ValueError):
                self.m.receipt_image(dict(pointer,**{key:value}),'222','d'*40)

    def test_review_refuses_stale_source_and_missing_rate_or_slot(self):
        import datetime as dt
        now=dt.datetime.now(dt.timezone.utc)
        review=dict(verifiedAt=now.isoformat(),expiresAt=(now+dt.timedelta(hours=1)).isoformat(),
                    sourceCommit='a'*40,releaseCommit='d'*40,releaseRun=222,budgetUsd=3,maxMinutes=120,
                    slot='root-reviewed-test-slot',rateSnapshotSha='e'*64,notBefore=(now-dt.timedelta(minutes=1)).isoformat())
        env=dict(self.env,BIBLE_RELEASE_COMMIT='d'*40,BIBLE_RELEASE_RUN='222',ISOLATION_RATE_SHA='e'*64)
        # Freeze is deliberately fixed to the scheduled media window; exercise other times deterministically.
        with __import__('unittest.mock',fromlist=['patch']).patch.object(self.m,'media_frozen',return_value=False):
            self.m.validate_approval(self.ctx,dict(env,BIBLE_ACA_ISOLATION_REVIEW=json.dumps(review)))
            for key,value in [('sourceCommit','f'*40),('rateSnapshotSha','f'*64),('slot',''),
                              ('verifiedAt',(now-dt.timedelta(hours=2)).isoformat())]:
                with self.subTest(key=key),self.assertRaises(ValueError):
                    self.m.validate_approval(self.ctx,dict(env,BIBLE_ACA_ISOLATION_REVIEW=json.dumps(dict(review,**{key:value}))))

    def test_provision_publishes_actual_identity_without_grant_or_apps(self):
        import tempfile
        cloud=FakeCloud(self.ctx)
        cloud.group,cloud.environment,cloud.role=None,None,None
        with tempfile.TemporaryDirectory() as folder:
            self.m.provision(self.ctx,cloud,self.m.plan(self.ctx,self.image,self.helper),Path(folder))
            review=json.loads((Path(folder)/'role-plan.json').read_text())
            self.assertEqual(review['principalId'],'11111111-1111-1111-1111-111111111111')
        self.assertEqual(cloud.written,[self.ctx.group_id,self.ctx.environment_id])
        self.assertNotIn('hhc-role-review',cloud.group['tags'])

    def test_arm_payload_is_stdin_and_error_never_emits_secret(self):
        from unittest.mock import patch
        result=subprocess.CompletedProcess([],1,stdout='secret-marker',stderr='secret-marker')
        with patch.object(self.m.subprocess,'run',return_value=result) as external:
            with self.assertRaises(RuntimeError) as error:
                self.m.Cloud(self.ctx).put(self.ctx.app_id('bible-ready-pg'),{'secret':'secret-marker'})
            call=external.call_args
            self.assertNotIn('secret-marker',str(call.args))
            self.assertIn('secret-marker',call.kwargs['input'])
            self.assertNotIn('secret-marker',str(error.exception))

    def test_bind_fatal_requires_same_event_and_correct_port(self):
        import tempfile
        with tempfile.TemporaryDirectory() as folder:
            path=Path(folder)/'daprd.log'
            path.write_text(json.dumps({'Log':'level=fatal failed listen tcp :50002: bind: address already in use'})+'\n')
            self.assertTrue(self.m.bind_fatal(path))
            path.write_text(json.dumps({'Log':'starting50002'})+'\n'+json.dumps({'Log':'level=fatal port3500 address already in use'})+'\n')
            self.assertFalse(self.m.bind_fatal(path))

    def test_inactive_running_revision_is_never_omitted_from_budget(self):
        import tempfile
        cloud=FakeCloud(self.ctx)
        cloud.get=lambda resource:{'properties':{'latestRevisionName':'old','latestReadyRevisionName':'old'}}
        cloud.cli=lambda *args:[{'name':'stray','properties':{'active':False,'replicas':1,'runningState':'Running'}}]
        with tempfile.TemporaryDirectory() as folder,self.assertRaises(ValueError):
            self.m.runtime_inventory(self.ctx,cloud,Path(folder),'preflight',{'old'})

    def test_helper_image_is_retained_if_resource_cleanup_failed(self):
        import tempfile
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as folder,patch.object(self.m,'Cloud',return_value=FakeCloud(self.ctx)),patch.object(self.m,'bounded',return_value='[]') as external:
            with self.assertRaises(ValueError):
                self.m.cleanup_helper(self.ctx,Path(folder))
            self.assertEqual(external.call_count,0)

    def test_all_transition_samples_need_nonce_revision_and_global_cadence(self):
        samples = [dict(phase='baseline', mono=i, nonce=str(i), status=200, valid=True, version=1+i%2) for i in range(3)]
        samples += [dict(phase='recover_transition', mono=i, nonce=str(i), status=200, valid=True, version=1+i%2) for i in range(3, 6)]
        phases = {'baseline': dict(seconds=1, revision='old'),
                  'recover_transition': dict(seconds=0, revisions=['old', 'new'])}
        routes = {s['nonce']: {'old'} for s in samples}
        self.assertEqual(self.m.assess(samples, routes, phases, True), 'PASS')
        for nonce, actual, result in [('4', None, 'UNKNOWN'), ('4', {'stray'}, 'FAIL'), ('4', {'old', 'new'}, 'FAIL')]:
            with self.subTest(actual=actual):
                changed = dict(routes, **{nonce: actual})
                self.assertEqual(self.m.assess(samples, changed, phases, True), result)
        # A gap between otherwise complete windows is also missing evidence.
        gap = copy.deepcopy(samples)
        for s in gap[3:]:
            s['mono'] += 10
        self.assertEqual(self.m.assess(gap, routes, phases, True), 'UNKNOWN')
        unknown = samples + [dict(samples[-1], phase='transition', nonce='extra', mono=6)]
        self.assertEqual(self.m.assess(unknown, dict(routes, extra={'stray'}), phases, True), 'UNKNOWN')

    def test_cleanup_rerun_checks_previous_attempt_and_never_reports_foreign_clean(self):
        first, second = FakeCloud(self.ctx), FakeCloud(self.m.Context(dict(self.env, GITHUB_RUN_ATTEMPT='2')))
        second.group, second.role = None, None
        clouds = {'123-1': first, '123-2': second}
        env = dict(self.env, GITHUB_RUN_ATTEMPT='2')
        with tempfile.TemporaryDirectory() as folder, patch.object(self.m, 'Cloud', side_effect=lambda ctx: clouds[ctx.run]):
            self.m.cleanup_attempts(env, Path(folder))
            self.assertEqual(first.deleted, [self.ctx.role_id, self.ctx.group_id])
            result = json.loads((Path(folder)/'cleanup.json').read_text())
            self.assertTrue(result['verifiedAbsent'])
            self.assertEqual(set(result['attempts']), {'123-1', '123-2'})
        first = FakeCloud(self.ctx)
        first.group['tags']['hhc-source'] = 'f'*40
        clouds['123-1'] = first
        with tempfile.TemporaryDirectory() as folder, patch.object(self.m, 'Cloud', side_effect=lambda ctx: clouds[ctx.run]):
            with self.assertRaises(ValueError):
                self.m.cleanup_attempts(env, Path(folder))
            self.assertFalse(json.loads((Path(folder)/'cleanup.json').read_text())['verifiedAbsent'])
            self.assertEqual(first.deleted, [])

    def test_helper_cleanup_rerun_includes_creating_attempt(self):
        with tempfile.TemporaryDirectory() as folder, patch.object(self.m, 'cleanup_helper', return_value={'ownedTagAbsent': True}) as clean:
            self.m.cleanup_attempts(dict(self.env, GITHUB_RUN_ATTEMPT='2'), Path(folder), helper=True)
            self.assertEqual([call.args[0].run for call in clean.call_args_list], ['123-1', '123-2'])

    def test_existing_helper_tag_refuses_build_before_any_write(self):
        rows = [{'name': 'isolation-123-1', 'digest': 'sha256:'+'c'*64}]
        with tempfile.TemporaryDirectory() as folder, patch.object(self.m, 'validate_approval'), \
                patch.object(self.m, 'helper_tags', return_value=rows), patch.object(self.m, 'bounded') as external:
            with self.assertRaises(ValueError):
                self.m.build_helper(self.ctx, Path(folder))
            external.assert_not_called()

    def test_helper_delete_demands_immutable_run_and_source_labels_and_exclusive_tag(self):
        cloud = FakeCloud(self.ctx)
        cloud.group, cloud.role = None, None
        sha = 'sha256:'+'c'*64
        row = {'name': 'isolation-123-1', 'digest': sha}
        own = {'hhc.isolation.run': self.ctx.run, 'org.opencontainers.image.revision': self.ctx.source}
        for labels in [{}, dict(own, **{'hhc.isolation.run': 'other'}), dict(own, **{'org.opencontainers.image.revision': 'f'*40})]:
            with self.subTest(labels=labels), tempfile.TemporaryDirectory() as folder, \
                    patch.object(self.m, 'Cloud', return_value=cloud), patch.object(self.m, 'helper_tags', return_value=[row]), \
                    patch.object(self.m, 'helper_labels', return_value=labels), patch.object(self.m, 'bounded') as external:
                with self.assertRaises(ValueError):
                    self.m.cleanup_helper(self.ctx, Path(folder))
                external.assert_not_called()
        with tempfile.TemporaryDirectory() as folder, patch.object(self.m, 'Cloud', return_value=cloud), \
                patch.object(self.m, 'helper_tags', return_value=[row, dict(row, name='shared')]), \
                patch.object(self.m, 'helper_labels', return_value=own), patch.object(self.m, 'bounded') as external:
            result = self.m.cleanup_helper(self.ctx, Path(folder))
            self.assertEqual(result['retainedDigest'], sha)
            external.assert_not_called()
        with tempfile.TemporaryDirectory() as folder, patch.object(self.m, 'Cloud', return_value=cloud), \
                patch.object(self.m, 'helper_tags', side_effect=[[row], [row], []]), \
                patch.object(self.m, 'helper_labels', return_value=own), patch.object(self.m, 'bounded', return_value='') as external:
            result = self.m.cleanup_helper(self.ctx, Path(folder))
            self.assertEqual(result['deletedDigest'], sha)
            self.assertIn('alive/bible-readiness-fixture@'+sha, external.call_args.args[0])

    def test_inventory_rejects_replica_growth_or_stray_during_measured_phase(self):
        cloud = FakeCloud(self.ctx)
        cloud.get = lambda resource: {'properties': {'latestRevisionName': 'old', 'latestReadyRevisionName': 'old'}}
        for rows in [[{'name': 'old', 'properties': {'active': True, 'replicas': 3, 'runningState': 'Running'}}],
                     [{'name': 'stray', 'properties': {'active': False, 'replicas': 1, 'runningState': 'Running'}}],
                     [{'name': 'old', 'properties': {'active': True, 'runningState': 'Running'}}]]:
            for row in rows:
                row['properties']['template'] = {'containers': [], 'scale': {'minReplicas': 1, 'maxReplicas': 3}}
            cloud.cli = lambda *args: rows
            with self.subTest(rows=rows), tempfile.TemporaryDirectory() as folder, self.assertRaises(ValueError):
                self.m.runtime_inventory(self.ctx, cloud, Path(folder), 'rollback-end', {'old'})

    def test_only_confirmed_retired_revision_can_close_log_stream(self):
        stopped = type('Stopped', (), {'poll': lambda self: 0})()
        rows = [{'name': 'old', 'properties': {'active': False, 'replicas': 0, 'runningState': 'Stopped'}}]
        self.m.validate_streams([(stopped, 'old')], rows)
        for properties in [dict(active=True, replicas=1, runningState='Running'),
                           dict(active=False, replicas=1, runningState='Running'),
                           dict(active=False, runningState='Stopped')]:
            with self.subTest(properties=properties), self.assertRaises(ValueError):
                self.m.validate_streams([(stopped, 'old')], [{'name': 'old', 'properties': properties}])
        with self.assertRaises(ValueError):
            self.m.validate_streams([(stopped, 'old')], [])

    def test_helper_labels_verifies_content_hashes_and_never_follows_redirects(self):
        import hashlib
        import io
        config = json.dumps({'config': {'Labels': {'hhc.isolation.run': self.ctx.run,
                              'org.opencontainers.image.revision': self.ctx.source}}}).encode()
        config_sha = 'sha256:'+hashlib.sha256(config).hexdigest()
        manifest = json.dumps({'config': {'digest': config_sha}}).encode()
        sha = 'sha256:'+hashlib.sha256(manifest).hexdigest()
        for blob, valid in [(config, True), (b'foreign config', False)]:
            opener = __import__('unittest.mock', fromlist=['Mock']).Mock()
            opener.open.side_effect = [io.BytesIO(b'{"access_token":"fake-pull-token"}'), io.BytesIO(manifest), io.BytesIO(blob)]
            with self.subTest(valid=valid), patch.object(self.m, 'bounded', return_value='{"accessToken":"fake-refresh-token"}'), \
                    patch.object(self.m.urllib.request, 'build_opener', return_value=opener) as build:
                if valid:
                    self.assertEqual(self.m.helper_labels(sha)['hhc.isolation.run'], self.ctx.run)
                else:
                    with self.assertRaises(ValueError):
                        self.m.helper_labels(sha)
                handler = build.call_args.args[0]
                self.assertIsNone(handler.redirect_request(None, None, 307, '', {}, 'https://foreign.invalid'))
                requests = [call.args[0] for call in opener.open.call_args_list]
                self.assertTrue(all(request.full_url.startswith('https://alive.azurecr.io/') for request in requests))
                self.assertIn(config_sha, requests[-1].full_url)


class FakeCloud:
    def __init__(self, ctx):
        self.ctx = ctx
        self.group = {'tags': {'hhc-owner': 'bible-readiness-isolation', 'hhc-run': '123-1',
                               'hhc-source': 'a'*40, 'hhc-expires': ctx.expires}}
        self.environment = {'identity': {'principalId': '11111111-1111-1111-1111-111111111111'}}
        self.role = {'properties': {'principalId': '11111111-1111-1111-1111-111111111111',
                                    'roleDefinitionId': ctx.pull_role}}
        self.deleted, self.written = [], []

    def get(self, resource):
        if resource == self.ctx.infrastructure_id:
            return None
        if resource == self.ctx.group_id:
            return self.group
        if resource == self.ctx.environment_id:
            return self.environment
        if resource == self.ctx.role_id:
            return self.role
        raise AssertionError(resource)

    def delete(self, resource):
        self.deleted.append(resource)
        if resource == self.ctx.group_id:
            self.group = None
        elif resource == self.ctx.role_id:
            self.role = None

    def put(self, resource, body):
        self.written.append(resource)
        if resource == self.ctx.role_id:
            self.role = body
        elif resource == self.ctx.group_id:
            self.group = body
        elif resource == self.ctx.environment_id:
            self.environment={'identity':{'principalId':'11111111-1111-1111-1111-111111111111'},
                              'properties':{'provisioningState':'Succeeded','infrastructureResourceGroup':self.ctx.infrastructure_group}}
        return body


if __name__ == '__main__':
    unittest.main()
