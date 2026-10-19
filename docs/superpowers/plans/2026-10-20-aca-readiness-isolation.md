# Bible ACA Readiness Isolation Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans inline. Source/PR/CI only in this phase; no Azure writes, environment creation, role grant, fault injection, merge or release.

**Goal:** Supply the missing isolated ACA acceptance workflow, with bounded candidate-only failure, evidence and cleanup.
**Architecture:** A fixed-target Python CLI generates manifests, previews, runs and cleans only a run-owned disposable environment. A small Go fixture supplies local shutdown/50002 hold/release and a Dapr caller. The Bible image is an immutable artifact from a successful normal main Release; production workflows/targets stay unchanged.
**Tech Stack:** Python standard library, Go standard library, Azure CLI, GitHub Actions.
**Spec:** `output/bible-dapr-readiness-20261020/aca-isolation-preview.md` in the website workspace and the coordinator's source-only implementation instruction.

## Global Constraints

- Fresh origin/main worktree; #10/#11 heads unchanged. Source has no production PG/scale/probe/API changes.
- Default preview, execute only trusted main with reviewed short-lived approval and assigned root slot; one fixed concurrency group.
- Dedicated East Asia Consumption RG/environment, synthetic PG, distinct Dapr IDs, no public ingress for Bible/caller, no production secrets/identity/DB/VNet.
- Source prep does not execute Azure mutations. Full heavyweight validation uses remote CI, protecting local media work.
- US$3/120min remains provisional estimate, not billing hard cap. Runtime100min plus independent always-cleanup20min; expiry/cleanup require evidence.

## Review Focus

1. Dynamic principal cannot be known before environment creation: dry-run marks unresolved; exact role/principal inventory must be reviewed before grant, never fabricate an ID.
2. Management failure, missing logs, no actual bind fatal or wrong-revision nonce cannot become PASS.
3. Helper admission race must not allow candidate through before port hold; unsupported managed shutdown is UNKNOWN and cleanup.
4. Cleanup must verify exact run ownership and only delete its own role/RG/helper artifact, including failure/cancellation and missing artifact state.
5. Main Release provenance, pool budget, request cadence and full old/failure/recovery/rollback windows must stay explicit.

## Task 1: Fixed manifests, guards and evidence assessment

Files: `scripts/aca-readiness-isolation/isolation.py`, `test_isolation.py`, README.
Interfaces: `Context.from_env`, `plan(context, bible_image, helper_image)`, `assess(samples, requests_by_revision, phases, bind_failure)`; pure preview before Azure writes, FAIL/UNKNOWN/PASS distinct.
- [ ] Tests first: reject wrong ref/run/subscription/image/target, manifest isolation, missing/failed/wrong-revision/cadence evidence and ownership cleanup.
- [ ] Implement fixed schema, source/digest attestation, sanitized preview, short-lived approval checks and run-owned exact cleanup.
- [ ] Run Python unit suite without real Azure and commit.

## Task 2: Candidate-only port fixture and Dapr caller

Files: `scripts/aca-readiness-isolation/fixture/{main.go,main_test.go}`, `Dockerfile`.
Interfaces: fixed localhost Dapr3500 and control8081; idle/hold/caller modes; GET state, POST release/phase; bounded nonce sample evidence. No fixture command executes production/host operations.
- [ ] Tests first: actual TCP hold/release after fake shutdown, unsupported API, admission readiness, bounded caller JSON and controls.
- [ ] Implement graceful shutdown then50002 hold; helper only ready after arm; release permits managed restart. Run focused Go tests with low parallelism; full race/build in CI.
- [ ] Commit source and tests.

## Task 3: Traceable workflow and disposable lifecycle

Files: `.github/workflows/readiness-aca-isolation.yml`, scoped CI step, isolation README and CLI lifecycle.
Interfaces: explicit preview/provision/execute/cleanup phases with run ownership, root-reviewed exact principal/role admission, main successful Release provenance and immutable artifacts; independent cleanup job reconstructs targets from trusted IDs.
- [ ] Fake CLI lifecycle tests first: no writes in preview, expired/wrong approval no grant, management failure preserves UNKNOWN, foreign RG/role never deleted, own resources removed after test failure.
- [ ] Implement bounded create/wait/log/exec/sample/recover/rollback/evidence path; fail closed when platform capability is absent. Workflow source dispatch does not execute during PR CI.
- [ ] Required CI includes Python/focused fixture tests and helper Docker build on remote runner; existing production checks remain intact.
- [ ] One independent full-branch review, one RED/GREEN fix pass for blockers; deferred minors recorded. Open PR and verify exact-head CI, stop before any Azure execution.
