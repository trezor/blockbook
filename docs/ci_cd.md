# CI/CD

## GitHub Actions Workflows

The repository currently uses two main workflows:

- `testing.yml` for automated test checks on pushes and pull requests
- `deploy.yml` for manual self-hosted build/deploy runs

## Testing Workflow

Workflow: `.github/workflows/testing.yml`

Trigger:

- `push` to `master` and `develop`
- `pull_request` to any branch

Jobs:

1. `unit-tests`
2. `connectivity-tests` test everything is reachable on the network
3. `integration-tests`

Security gate for self-hosted test jobs:

- self-hosted jobs run only for non-PR events or same-repository PRs
- condition:
  `github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository`

## Deploy Workflow

Workflow: `.github/workflows/deploy.yml`

Trigger:

- manual `workflow_dispatch`

Inputs:

- `mode`: 
  - `build` when you want to build Blockbook Debian packages only.
  - `deploy`
  - when you want the full flow:
    1. build package
    2. install and restart service
    3. wait for Blockbook sync
    4. run post-deploy e2e tests
- `coins`: comma-separated aliases from `configs/coins` or `ALL`
- `ref`: optional checkout/deploy ref; leave empty to use the workflow run ref

## Trigger from `gh` CLI

Examples assume the workflow file already exists on the selected workflow branch.

Build selected coins:

```bash
gh workflow run deploy.yml --ref <workflow-branch> -f mode='build' -f coins='bitcoin,dogecoin'
```

Deploy selected coins:

```bash
gh workflow run deploy.yml --ref <workflow-branch> -f mode='deploy' -f coins='bitcoin,dogecoin'
```

Deploy with explicit checkout ref:

```bash
gh workflow run deploy.yml --ref <workflow-branch> -f mode='deploy' -f coins='bitcoin' -f ref='<commit-or-branch>'
```

Build all mapped coins:

```bash
gh workflow run deploy.yml --ref <workflow-branch> -f mode='build' -f coins='ALL'
```

Deploy all mapped coins:

```bash
gh workflow run deploy.yml --ref <workflow-branch> -f mode='deploy' -f coins='ALL'
```

Monitor runs:

```bash
gh run list --workflow deploy.yml --limit 5
gh run watch <run-id>
gh run view <run-id> --log
```

Ref behavior:

- `--ref` chooses which branch/tag contains the workflow definition
- `ref` chooses what commit/branch/tag the jobs actually check out and deploy
