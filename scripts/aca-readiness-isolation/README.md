# Disposable ACA readiness acceptance

This tool is preparation for Bible's separate production probe change. It does
not modify the production Release workflow, production PG, replicas, probes,
Gateway, Dapr settings, identities or application routes. Merging this source
does not trigger Production Release under its existing path filter. Never
execute the isolation workflow from a PR or use an unmerged Bible artifact.

The workflow defaults to a read-only resource-intent preview. It validates a
successful normal main `.github/workflows/release.yml` run and its main commit,
then reads that exact run/commit's immutable image from the production OpenAPI
Release receipt, using the existing docs identity for read-only access. A moved
current receipt fails rather than falling back to a mutable main tag. The preview's helper
digest is explicitly zero/unbuilt. This is not an Azure provider what-if or
proof that the requested resources will provision. Actual execution additionally
requires a fresh coordinator review, assigned slot and a real helper digest
built from this workflow's merged main source.

## Execution prerequisites

The referenced normal main Release must already have accepted production smoke
and fresh artifact/release-boundary checks. Never allocate during an active
media freeze. A coordinator may assign a slot alongside an existing media job
only after recording fresh quota/headroom, budget and cleanup capacity, and
confirming this disposable environment and fixture DB share no production
runtime or data dependency with that job. Otherwise keep the cohorts serial.
This scenario validates readiness routing; it is not a fix for a running
managed sidecar failure.

The existing OIDC configuration is reused; its actual permissions for creating
the new RG/environment, building the helper, and managing the exact registry
role must be reviewed first. No permission broadening is implemented here. If
the existing identity cannot perform an operation, stop and review a scoped
execution identity/permission plan; never widen production access to make this
workflow pass. No Key Vault, Blob, production DB or existing managed identity
is assigned to the disposable apps.

Use preview to inspect `retail-rates.json` and its SHA256, and set the nonsecret
repository variable `BIBLE_ACA_ISOLATION_REVIEW` only after exact review:

```json
{
  "verifiedAt": "<UTC ISO time, at most30 minutes old>",
  "expiresAt": "<UTC ISO time, no more than2 hours ahead>",
  "notBefore": "<assigned coordinator slot UTC ISO start>",
  "sourceCommit": "<exact merged isolation workflow40hex SHA>",
  "releaseCommit": "<successful normal Bible Release40hex SHA>",
  "releaseRun": "<that Release run ID>",
  "rateSnapshotSha": "<SHA256 of canonical selected fresh retail meters>",
  "budgetUsd": 3,
  "maxMinutes": 120,
  "slot": "<coordinator allocation reference>"
}
```

US$3/120min is the provisional incremental Azure estimate, not a billing hard
cap. Free grants/discounts are not assumed; ACR helper build/storage, managed
environment/sidecar uncertainty and failed cleanup require separate evidence.
GitHub overage, taxes and FX are not certified. Price changes fail the review
hash check. The100min main job includes helper build, environment provisioning,
the role review wait and test; cleanup has a separate20min `always` job.

Execution creates only `hhc-bible-ready-<run>-<attempt>` and the declared
platform infrastructure group, a Consumption environment, synthetic pgvector
PG17 container, target and caller. Target Single mode and0.25CPU/0.5Gi match production, with an isolated
min1/max3 test limit (production max8); its test helper adds0.25CPU/0.5Gi. PG/caller each
stay one replica. The fixture PG is not `hhc-pg`; it has ephemeral data, a
run-generated secret and a100connection budget. No public ingress is used by
target/caller; PG has internal TCP only. Logs destination none avoids a new
workspace; missing streaming evidence remains UNKNOWN.

## Exact principal review during the assigned run

Provision creates the owned RG/environment **without** AcrPull, then publishes
`role-plan.json` before the test step. The new principal cannot be known before
this stage. The coordinator verifies the exact environment identity, registry
scope, deterministic roleAssignment ID and source/run from that artifact.

The coordinator may then set `hhc-role-review=<canonical SHA256 of role-plan.json>`
on that exact disposable RG using its normal reviewed Azure tooling. The
canonical hash is printed by provision and uses sorted compact JSON, so a raw
file SHA is not interchangeable. The executor never writes that marker. It
waits at most600seconds and re-reads the actual principal before granting only
the reviewed role. Missing/stale/mismatched review stops and cleans. This tag
is a coordination/audit marker, not an Azure security lock. No routine human
confirmation is needed beyond the already authorized coordinator decision.

### Coordinator execution

If the CI identity lacks isolation permissions, the authorized coordinator may
run these same commands with its existing Azure login from a clean checkout of
the exact merged main commit. Record this as coordinator execution, not a CI
run. Use a real preview workflow run/attempt for resource naming; verify the
checkout and release receipt independently because the `GITHUB_*` metadata is
not authorization. Both Linux and macOS console transports preserve command
failure status.

Keep `GITHUB_ENV` private and propagate its generated values between commands.
All fresh slot, budget, principal-review and resource-ownership checks still
apply. Run `cleanup` and `cleanup-helper` after success or failure, and reconcile
interrupted cleanup before closing the run. Do not widen the production CI
identity or substitute an unmerged production image.

## What is measured

The acceptance contract is explicitly layered:

1. The existing local Docker test injects a real Dapr50002 bind fault and checks
   Bible `/health`200, `/ready`503 and recovery. It does not certify managed ACA.
2. `aca-readiness-gate-v1` uses actual ACA Single mode and the same Dapr appId.
   The original Bible probes remain in place. A candidate's isolated helper
   holds its own readiness endpoint at503 until a loopback-only release. Bible
   `/health` and `/ready` must both stay200 and Dapr outbound health must stay204;
   no socket is held and no shutdown API is called by this ACA scenario.

The candidate must remain unready while the old revision serves every request.
Opening the helper gate then proves actual readiness routing/recovery, followed
by a second closed candidate and full original-template rollback. Every closed
window requires continuous600second observations of the closed helper, healthy
Bible and healthy Dapr. Missing or contradictory evidence remains UNKNOWN.
Each candidate poll saves its decoded controlled fixture state before checking
capability, preserving the rejection reason and observations after cleanup.

These layers can support the separate normal readiness-probe delivery, with a
retained limitation: they do not prove routing, drain, discovery or restart
behavior during an actual managed-daprd bind failure. Acceptance artifacts name
the scenario and set `actualManagedBindFailure` to `NOT_TESTED`; no prior failed
or UNKNOWN allocation is reclassified. ACA does not list the OSS shutdown API
among supported operational APIs, so this tool does not retry that injection.
There is no force, wrong-port fallback, privileged container or host access.

The caller continuously requests v1/v2 versions with unique synthetic nonce
queries and validates the single sentinel record. It records version, status, hash,
monotonic time and phase. Both versions must appear in each measured window.
Phase acknowledgements wait for the preceding request
to finish. Exact Bible application revision/replica/container log streams associate each nonce
with the serving revision; 200 without that association is insufficient.
Missing logs/403/429/management failure/cadence gaps are UNKNOWN; observed
HTTP/JSON failures or wrong-revision routing are FAIL.

Windows: baseline60s, closed-gate candidate600s, recovered60s, another closed-gate
candidate600s, restored original-template60s. Requests continue through
transitions. Preparation and handoff have separate phase identities: preparation
allows only the old serving revision; recovery/rollback allow only their exact
old/new pair. Every sample, including transitions, needs one nonce/revision
association and the global cadence check; an unrecognized phase is UNKNOWN.
The final rollback copies the original idle-helper template,
not the dynamically released hold-helper revision. Template/digest equality,
inventory including inactive-but-running revisions, at most three revision
groups and actual old PG application_name connections returning to zero are
checked. Every window boundary checks an explicit live revision allowlist and
known zero/one replica counts, including inactive revisions. An unexpected
additional replica stops this one-replica acceptance;
it is not peak/soak or max3 load certification.
Log streams may close only after the exact revision is confirmed inactive,
stopped and at zero replicas; loss of an active or unknown stream is UNKNOWN.
All requests still require captured nonce evidence after a legitimate retirement.

## Cleanup and remaining live gate

Cleanup reconstructs exact IDs for every attempt from one through the current
trusted workflow run attempt, even if no artifact survived. A cleanup-only
rerun therefore checks the original creating attempt too. Each attempt is
checked separately; a foreign or failed allocation prevents an aggregate clean
result while other attempts are still checked. It verifies owner/run/source/expiry tags, matches the
registry role to the new environment principal, removes only that role, then
deletes the exact owned RG and confirms both it and the declared platform
infrastructure RG are absent. It refuses foreign groups/roles or an orphan
role whose principal cannot be attested. Residual infrastructure is reported
for exact follow-up, never wildcard-deleted or declared clean.

Helper builds refuse an existing run-specific tag. Cleanup verifies the actual
immutable manifest/config SHA256 and both run/source labels before deletion,
then rechecks the tag/digest and exclusive tag reference. It deletes that digest
only when no other tag shares it; otherwise it records retained storage.
Missing or foreign ownership is UNKNOWN and retains the image. Registry pull
tokens stay in memory; redirects are refused rather than forwarding credentials.
The bounded reader supports a direct image manifest; unsupported image indexes
or registry redirect behavior retain the artifact for exact follow-up.
The reader follows the [ACR OAuth contract](https://github.com/Azure/acr/blob/main/docs/AAD-OAuth.md)
and [OCI manifest/blob digest contract](https://github.com/opencontainers/distribution-spec/blob/main/spec.md).
Cancelled remote ACR builds, lost runners, unavailable role reads or an
unscheduled cleanup job still require the coordinator's expiry/inventory
check. Tags are not automatic TTL deletion and actual charges continue until
resources are gone. Do not delete production Bible images or broadly purge
the registry. Preserve unmerged worktrees and evidence until live acceptance.

Local light checks:

```sh
python3 -m unittest discover -s scripts/aca-readiness-isolation -p 'test_*.py'
go test ./scripts/aca-readiness-isolation/fixture -count=1 -p=1
```

Required remote CI also runs the repository race/vet/OpenAPI/release checks and
builds both images. None of these execute ACA. Even after source/CI pass, an
assigned real isolated `aca-readiness-gate-v1` run with full evidence and cleanup
must PASS before the separate normal production probe delivery. The local Dapr
fault suite must also pass; managed-Dapr fault timing remains explicitly untested.

References: [ACA supported APIs and limitations](https://learn.microsoft.com/en-us/azure/container-apps/dapr-overview#limitations),
[OSS asynchronous shutdown contract](https://docs.dapr.io/reference/api/shutdown_api/).
