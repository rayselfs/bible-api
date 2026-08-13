# Retire Bible Vector Job Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Permanently stop rebuilding PostgreSQL Bible vectors while leaving all database schema and data unchanged.

**Architecture:** Remove the image producer before deleting the scheduled consumer and its ACR repository. Keep `bible-api`, `bible-import`, PostgreSQL tables, migrations, and API routes unchanged.

**Tech Stack:** Azure DevOps YAML, Azure Container Apps Jobs, Azure Container Registry, Go.

## Global Constraints

- Do not change PostgreSQL tables, data, or migrations.
- Do not change or delete `bible-import`.
- Merge the repository change before deleting the Azure Job and ACR repository.
- Preserve all unrelated untracked documentation in the checkout.

---

### Task 1: Remove the vector image producer

**Files:**
- Modify: `.azure-devops/azure-pipelines.yml`
- Delete: `scripts/Dockerfile`
- Delete: `scripts/generate_vectors.py`

**Interfaces:**
- Consumes: tag-triggered Azure DevOps pipeline.
- Produces: a pipeline that only builds and pushes `alive/bible-api`.

- [ ] **Step 1: Verify the obsolete producer still exists**

Run:

```bash
rg -n 'scriptRepository|Build Script Gen|Push Script Gen|generate_vectors' .azure-devops scripts
```

Expected: matches in the pipeline, Dockerfile, and Python script.

- [ ] **Step 2: Remove the producer**

Delete the `scriptRepository` variable and both script-image Docker tasks from
`.azure-devops/azure-pipelines.yml`. Delete `scripts/Dockerfile` and
`scripts/generate_vectors.py`.

- [ ] **Step 3: Verify no recreation path remains**

Run:

```bash
if rg -n 'scriptRepository|alive/bible-script|generate_vectors' .azure-devops scripts; then exit 1; fi
```

Expected: exit 0 with no matches.

- [ ] **Step 4: Verify the unaffected application**

Run:

```bash
go test ./...
go vet ./...
ruby -e 'require "yaml"; YAML.load_file(".azure-devops/azure-pipelines.yml")'
git diff --check
```

Expected: every command exits 0.

- [ ] **Step 5: Commit and publish through a PR**

Commit only the pipeline and deleted script files, push the focused branch,
create an Azure DevOps PR to `main`, then squash-complete it after required
policies pass.

### Task 2: Delete the scheduled Job and images

**Files:** None.

**Interfaces:**
- Consumes: merged pipeline change from Task 1.
- Produces: no `bible-script` Job and no `alive/bible-script` ACR repository.

- [ ] **Step 1: Capture a fresh deletion preview**

Verify the Job is scheduled, no execution is running, the image is
`alive.azurecr.io/alive/bible-script:v1.3.5`, and the repository contains seven
manifests.

- [ ] **Step 2: Delete the Job before the images**

Run:

```bash
az containerapp job delete -g alive -n bible-script --yes
az acr repository delete -n alive --repository alive/bible-script --yes
```

- [ ] **Step 3: Verify deletion and unaffected routes**

Require the Job and repository to be absent. Require HTTP 200 from
`/health`, `/ready`, `/api/bible/v1/versions`, and a known Bible content route.

