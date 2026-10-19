import copy
import importlib.util
from pathlib import Path
import unittest

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

    def test_context_rejects_untrusted_ref_repository_and_noncanonical_ids(self):
        for key, bad in [('GITHUB_REF', 'refs/heads/feature'), ('GITHUB_REPOSITORY', 'fork/bible-api'),
                         ('GITHUB_RUN_ID', '../alive'), ('GITHUB_RUN_ID', '01'),
                         ('GITHUB_RUN_ATTEMPT', '0'), ('AZURE_SUBSCRIPTION_ID', 'other')]:
            with self.subTest(key=key, bad=bad), self.assertRaises(ValueError):
                self.m.Context(dict(self.env, **{key: bad}))
        self.assertEqual(self.ctx.group, 'hhc-bible-ready-123-1')

    def test_manifest_isolates_identity_database_dapr_and_ingress(self):
        plan = self.m.plan(self.ctx, self.image, self.helper)
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
                        valid=True) for i in range(602)]
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
        return body


if __name__ == '__main__':
    unittest.main()
