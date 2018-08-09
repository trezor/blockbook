# Blockbook Contributor Guide

Blockbook is back-end service for Trezor wallet. Although it is open source, design and development of core packages
is done by Trezor developers in order to keep Blockbook compatible with Trezor. If you feel you could use Blockbook
for another purposes, we recommend you to make a fork.

However you can still help us find bugs or add support for new coins.

## Development environment

Instructions to set up your development environment and build Blockbook are described in separated
[document](/docs/build.md).

## How can I contribute?

### Reporting bugs

### Adding coin support

Trezor harware wallet supports over 500 coins, see https://trezor.io/coins/. You are free to add support for any of
them to Blockbook. Actually implemented coins are listed [here](/docs/ports.md).

You should follow few steps bellow to get smooth merge of your PR.

> Altough we are happy for support of new coins we have not enough capacity to run them all on our infrastructure.
> Actually we can run Blockbook instances only for coins supported by Trezor wallet. If you want to have Blockbook
> instance for your coin, you will have to deploy your own server.

#### Add coin definition

Coin definitions are stored in JSON files in *configs/coins* directory. They are single source of Blockbook
configuration, Blockbook and back-end package definition and build metadata. Since Blockbook supports only single
coin index per running instance, every coin (including testnet) must have single definition file.

All options of coin definition are described in [config.md](/docs/config.md).

Because most of coins are fork of Bitcoin and they have similar way to install and configure their daemon, we use
templates to generate package definition and configuration files during build process. It is similar to build Blockbook
package too. Templates are filled with data from coin definition. Although build process generate packages
automatically, there is sometimes necessary see intermediate step. You can generate all files by calling
`go run build/templates/generate.go coin` where *coin* is name of definition file without .json extension. Files are
generated to *build/pkg-defs* directory.

Good examples of coin configuration are
[*configs/coins/bitcoin.json*](configs/coins/bitcoin.json) and
[*configs/coins/ethereum.json*](configs/coins/ethereum.json) for Bitcoin-like coins and different coins, respectively.

Usually you have to update only few options that differ from Bitcoin definition. At first there are base information
about coin in section *coin* – name, alias etc. Then update port information in *port* section. We keep port series as
listed in [port registry](/docs/ports.md). Select next port numbers in series. Port numbers must be unique across all
port definitions.

In section *backend* update information how to build and configure backend service. When back-end package is built,
build process downloads installation archive, verify and extract it. How it is done is described in
[build guide](/docs/build.md#on-back-end-building). Naming conventions and versioning are described
also in [build guide](/docs/build.md#on-naming-conventions-and-versioning). You have to update *package_name*,
*package_revision*, *system_user*, *version*, *binary_url*, *verification_type*, *verification_source*, *extract_command* and
*exclude_files*. Also update information whether service runs mainnet or testnet network in *mainnet* option.

In section *blockbook* update information how to build and configure Blockbook service. Usually they are only
*package_name*, *system_user* and *explorer_url* options. Naming conventions are are described
[here](/docs/build.md#on-naming-conventions-and-versioning).

Update *package_maintainer* and *package_maintainer_email* options in section *meta*.

Execute script *contrib/scripts/generate-port-registry.go* that will update *docs/ports.md*.

Now you can try generate package definitions as described above in order to check outputs.

TODO:
* script that checks unique port numbers

#### Add coin implementation

#### Add tests

#### Deploy public server
