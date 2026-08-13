# Bible Vector Job Retirement And ACR Hardening

## Goal

Retire the unused PostgreSQL-vector regeneration path and complete the ACR
credential hardening without interrupting the Bible API or API gateway.

## Scope

### Bible vector job retirement

- Remove the Azure Container Apps Job `bible-script`.
- Delete the complete ACR repository `alive/bible-script` after the Job is gone.
- Remove `scriptRepository` and the script-image build/push steps from the
  `bible-api` Azure DevOps pipeline.
- Delete `scripts/Dockerfile` and `scripts/generate_vectors.py`.
- Do not change PostgreSQL tables, data, migrations, Bible API handlers, or the
  separate `bible-import` Job.

### ACR authentication hardening

- Give `api-gateway` a system-assigned managed identity with `AcrPull` on the
  `alive` registry.
- Replace its username/password registry configuration with that identity.
- Add a release preflight in the `api-gateway` repository that rejects a
  password-based ACR configuration.
- Merge and release through the existing GitHub Actions workflow. A healthy new
  revision and the existing route smoke tests prove the managed-identity pull.
- Only after that release succeeds, remove the unused registry secret and
  disable the ACR admin user.

## Order Of Operations

1. Change `bible-api` on a focused branch and merge its PR so a future tag
   cannot recreate `alive/bible-script`.
2. Delete the `bible-script` Job, then delete its ACR repository.
3. Change `api-gateway` on a separate focused branch to add the registry-auth
   preflight.
4. Configure the gateway managed identity and `AcrPull` assignment while the
   ACR admin user remains enabled.
5. Merge the gateway PR and let CI/CD build, deploy, and smoke-test a new
   immutable revision.
6. Remove the old registry secret, disable the ACR admin user, and repeat the
   live health and route checks.

## Rollback

- Bible retirement stops before deleting the Job if the pipeline change is not
  merged. The database remains untouched throughout.
- Gateway hardening stops before disabling the admin user unless a new revision
  has pulled through managed identity and passed smoke tests.
- If gateway identity setup fails, keep the current active revision and restore
  the previous registry configuration while the admin user is still enabled.

## Verification

- `bible-script` is absent from Container Apps Jobs and Azure Resource Graph.
- `alive/bible-script` is absent from ACR.
- The Bible API health, versions, and content routes return HTTP 200.
- `api-gateway` has a system-assigned identity, an `AcrPull` assignment, and no
  registry password reference.
- ACR admin is disabled only after a new gateway revision is healthy.
- Public website, account, admin, Bible, OIDC, and bulletin smoke routes pass.

