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
  - `prod` builds selected coins on `production_builder` regardless of `BB_RUNNER_*`
  - default is `dev`
  - ignored when `mode=deploy`
- `coins`: comma-separated aliases from `configs/coins`; `ALL` is supported only in `mode=build`
- `checkout_ref`: optional checkout/deploy ref; leave empty to use the workflow run ref

In `mode=build`, selected coins are grouped by runner so one build job can build multiple
`deb-blockbook-<coin>` targets in a single invocation on the same self-hosted machine.

Special cases:

- `mode=build` + `env=dev` skips prod-only coins when `coins=ALL`
- `mode=build` + `env=prod` + `coins=ALL` builds all configured coins with `BB_RUNNER_*` mappings on `production_builder`
- `mode=build` + `env=dev` fails if you explicitly request a coin whose `BB_RUNNER_*` is `production_builder`
- `mode=deploy` is dev-only and fails fast if any selected coin is mapped to `production_builder`

## CLI examples

Without `--run`, `build` and `deploy` print the underlying `gh workflow run ...`
command. `list` prints coins, not commands.

Current branch example output was captured on `new-test-name-config`, so the printed
`--ref` and `checkout_ref` values will differ on other branches.
The output below assumes `BB_RUNNER_*` repository variables are valid for the current checkout.

List coins buildable on dev runners:

```bash
./.github/scripts/run.py list --env dev
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
./.github/scripts/run.py list --env prod --format csv
```

```text
arbitrum_archive,avalanche_archive,base_archive,bcash,bitcoin,bitcoin_regtest,bitcoin_testnet,bitcoin_testnet4,bsc_archive,dash,dogecoin,ethereum_archive,ethereum_testnet_hoodi_archive,ethereum_testnet_sepolia_archive,litecoin,optimism_archive,polygon_archive,zcash
```

Print the default dev build command for selected coins:

```bash
./.github/scripts/run.py build --coins bitcoin,dogecoin
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=build -f env=dev -f coins=bitcoin,dogecoin -f checkout_ref=new-test-name-config
```

Print the prod build command for selected coins:

```bash
./.github/scripts/run.py build --env prod --coins bitcoin,bsc_archive
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=build -f env=prod -f coins=bitcoin,bsc_archive -f checkout_ref=new-test-name-config
```

Print the dev build command for all selectable coins:

```bash
./.github/scripts/run.py build --coins ALL
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=build -f env=dev -f coins=ALL -f checkout_ref=new-test-name-config
```

Print the prod build command for all selectable coins:

```bash
./.github/scripts/run.py build --env prod --coins ALL
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=build -f env=prod -f coins=ALL -f checkout_ref=new-test-name-config
```

Print the deploy command for selected coins:

```bash
./.github/scripts/run.py deploy --coins bitcoin,dogecoin
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=deploy -f env=dev -f coins=bitcoin,dogecoin -f checkout_ref=new-test-name-config
```

Print the deploy command with an explicit checkout ref:

```bash
./.github/scripts/run.py deploy --coins bitcoin --checkout-ref master
```

```text
gh workflow run deploy.yml -R trezor/blockbook --ref new-test-name-config -f mode=deploy -f env=dev -f coins=bitcoin -f checkout_ref=master
```
