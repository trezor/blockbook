# CI/CD

## GitHub Actions Workflows

The repository currently uses two main workflows:

- `testing.yml` for automated test checks on pushes and pull requests
- `deploy.yml` for manual self-hosted build/deploy runs (shown in GitHub Actions as `Build / Deploy`)

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

Workflow: `.github/workflows/deploy.yml` (`Build / Deploy` in the Actions UI)

Trigger:

- manual `workflow_dispatch`

Inputs:

- `mode`: 
  - `build` when you want to build Blockbook Debian packages only.
  - `deploy` when you want the full flow:
    1. build package
    2. install and restart service
    3. wait for Blockbook sync
    4. run post-deploy e2e tests
- `env`:
  - `dev` keeps the current per-coin dev runner mapping
  - `prod` builds selected coins on the `production-builder` runner regardless of `BB_RUNNER_*`
  - default is `dev`
  - selected value is exported downstream as `BB_BUILD_ENV`
  - ignored when `mode=deploy`
- `always_build_backend`:
  - `false` derives backend builds per coin from the selected `BB_{DEV|PROD}_RPC_URL_HTTP_<coin_alias>` value
  - backend is built when that env var is unset, empty, or resolves to `localhost`, `127.0.0.1`, or `::1`
  - backend is skipped only when the env var is present and points to a non-loopback target
  - `true` forces backend builds for all selected coins
  - ignored when `mode=deploy`
- `coins`: comma-separated aliases from `configs/coins`; `ALL` is supported only in `mode=build`
- `branch_or_tag`: optional branch or tag to check out and deploy; leave empty to use the workflow run ref name
  - the selected value is validated before checkout and must exist in the target repository as a branch or tag

In `mode=build`, selected coins are grouped by runner so one build job can build multiple
`deb-blockbook-<coin>` targets in a single invocation on the same self-hosted machine.
Deploy and test-related workflow steps use `BB_BUILD_ENV=dev`.

Env vars :

See also [CI/CD workflow variables](env.md#cicd-workflow-variables).

- `BB_PACKAGE_ROOT=/opt/blockbook-builds`
  - When absolute path set, build jobs copy packages to:
  - `/opt/blockbook-builds/{branch_or_tag}/{coin}/blockbook-*.deb`
  - `/opt/blockbook-builds/{branch_or_tag}/{coin}/backend-*.deb`
  - `{coin}` here is the workflow/config name from `configs/coins/<coin>.json`, not `coin.alias`

Special cases:

- `mode=build` + `env=dev` skips prod-only coins when `coins=ALL`
- `mode=build` + `env=prod` + `coins=ALL` builds all configured coins with `BB_RUNNER_*` mappings on the `production-builder` runner
- `mode=build` + `env=dev` fails if you explicitly request a coin whose `BB_RUNNER_*` is `production_builder`
- `mode=deploy` is dev-only and fails fast if any selected coin is mapped to `production_builder`

## Naming Matrix

```text
+-------------------------------+----------------------------------------+--------------------------------------+
| Concern                       | Example source                         | Name used                            |
+-------------------------------+----------------------------------------+--------------------------------------+
| Workflow/build/deploy identity| configs/coins/<coin>.json filename     | polygon_archive                      |
| Runner mapping                | BB_RUNNER_<coin>                       | BB_RUNNER_POLYGON_ARCHIVE            |
| Build env selector            | BB_BUILD_ENV                           | dev                                  |
| Backend RPC env identity      | coin.alias                             | BB_DEV_RPC_URL_HTTP_polygon_archive_bor |
| Blockbook package name        | blockbook.package_name                 | blockbook-polygon                    |
| Backend package name          | backend.package_name                   | backend-polygon                      |
| Build target identity         | workflow/config coin name              | deb-blockbook-polygon_archive        |
| Built Blockbook .deb filename | build/<blockbook.package_name>_*.deb   | build/blockbook-polygon_*.deb        |
| Built backend .deb filename   | build/<backend.package_name>_*.deb     | build/backend-polygon_*.deb          |
| Staged artifact path identity | workflow/config coin name              | {branch_or_tag}/polygon_archive/...  |
| API/e2e test identity         | coin.test_name or config filename      | polygon                              |
| API test env identity         | BB_DEV_API_URL_* from test identity    | BB_DEV_API_URL_HTTP_polygon          |
+-------------------------------+----------------------------------------+--------------------------------------+
```

For `polygon_archive` specifically:

- workflow coin: `polygon_archive`
- alias: `polygon_archive_bor`
- blockbook package name: `blockbook-polygon`
- backend package name: `backend-polygon`
- test name: `polygon`

## CLI examples

Wrapper entrypoint:

```bash
./.github/bin/bb_deploy
```

Without `--run`, `build` and `deploy` print the underlying `gh workflow run ...`
command. `list` prints coins, not commands.

Current branch example output was captured on `new-test-name-config`, so the printed
`--ref` and `branch_or_tag` values will differ on other branches.
The output below assumes `BB_RUNNER_*` repository variables are valid for the current checkout.

List coins buildable on dev runners:

```bash
./.github/bin/bb_deploy list --env dev
```

```text
avalanche_archive
base_archive
bcash
bitcoin
bitcoin_regtest
bitcoin_testnet
bitcoin_testnet4
bsc_archive
dash
dogecoin
ethereum_archive
ethereum_testnet_hoodi_archive
ethereum_testnet_sepolia_archive
litecoin
zcash
```

List all configured runner-mapped coins in CSV form:

```bash
./.github/bin/bb_deploy list --env prod --format csv
```

```text
arbitrum_archive,avalanche_archive,base_archive,bcash,bitcoin,bitcoin_regtest,bitcoin_testnet,bitcoin_testnet4,bsc_archive,dash,dogecoin,ethereum_archive,ethereum_testnet_hoodi_archive,ethereum_testnet_sepolia_archive,litecoin,optimism_archive,polygon_archive,zcash
```

Print the default dev build command for selected coins:

```bash
./.github/bin/bb_deploy build --coins bitcoin,dogecoin
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=build -f env=dev -f coins=bitcoin,dogecoin -f branch_or_tag=new-test-name-config
```

Print the prod build command for selected coins:

```bash
./.github/bin/bb_deploy build --env prod --coins bitcoin,bsc_archive
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=build -f env=prod -f coins=bitcoin,bsc_archive -f branch_or_tag=new-test-name-config
```

Print the dev build command for all selectable coins:

```bash
./.github/bin/bb_deploy build --coins ALL
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=build -f env=dev -f coins=ALL -f branch_or_tag=new-test-name-config
```

Print the prod build command for all selectable coins:

```bash
./.github/bin/bb_deploy build --env prod --coins ALL
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=build -f env=prod -f coins=ALL -f branch_or_tag=new-test-name-config
```

Print the deploy command for selected coins:

```bash
./.github/bin/bb_deploy deploy --coins bitcoin,dogecoin
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=deploy -f env=dev -f coins=bitcoin,dogecoin -f branch_or_tag=new-test-name-config
```

Print the deploy command with an explicit branch or tag:

```bash
./.github/bin/bb_deploy deploy --coins bitcoin --branch-or-tag master
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=deploy -f env=dev -f coins=bitcoin -f branch_or_tag=master
```
