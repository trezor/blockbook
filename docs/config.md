# Configuration

Coin definitions are stored in JSON files in *configs/coins* directory. They are single source of Blockbook
configuration, Blockbook and back-end package definition and build metadata. Since Blockbook supports only single
coin index per running instance, every coin (including testnet) must have single definition file.

Because most of coins are fork of Bitcoin and they have similar way to install and configure their daemon, we use
templates to generate package definition and configuration files during build process. It is similar to build Blockbook
package too. Templates are filled with data from coin definition. Although build process generate packages
automatically, there is sometimes necessary see intermediate step. You can generate all files by calling
`go run build/templates/generate.go coin` where *coin* is name of definition file without .json extension. Files are
generated to *build/pkg-defs* directory.

Good examples of coin configuration are
[*configs/coins/bitcoin.json*](/configs/coins/bitcoin.json) and
[*configs/coins/ethereum.json*](/configs/coins/ethereum.json) for Bitcoin-like coins and different coins, respectively.

## Description of coin definition

* `coin` – Base information about coin.
    * `name` – Name of coin used internally (e.g. "Bcash Testnet").
    * `shortcut` – Ticker symbol (code) of coin (e.g. "TBCH").
    * `label` – Name of coin used publicly (e.g. "Bitcoin Cash Testnet").
    * `alias` – Name of coin used in file system paths and config files. We use convention that name uses lowercase
       characters and underscore '_' as a word delimiter. Testnet versions of coins must have *_testnet*
      suffix. For example "bcash_testnet".

* `ports` – List of ports used by both back-end and Blockbook. Ports defined here are used in configuration templates
   and also as source for generated documentation.
    * `backend_rpc` – Port of back-end RPC that is connected by Blockbook service.
    * `backend_message_queue` – Port of back-end MQ (if used) that is connected by Blockbook service.
    * `backend_*` – Additional back-end ports can be documented here. Actually the only purpose is to get them to
       port table (prefix is removed and rest of string is used as note).
    * `blockbook_internal` – Blockbook's internal port that is used for metric collecting, debugging etc.
    * `blockbook_public` – Blockbook's public port that is used to communicate with Trezor wallet (via Socket.IO).

* `ipc` – Defines how Blockbook connects its back-end service.
    * `rpc_url_template` – Template that defines URL of back-end RPC service. See note on templates below.
    * `rpc_user` – User name of back-end RPC service, used by both Blockbook and back-end configuration templates.
    * `rpc_pass` – Password of back-end RPC service, used by both Blockbook and back-end configuration templates.
    * `rpc_timeout` – RPC timeout used by Blockbook.
    * `message_queue_binding_template` – Template that defines URL of back-end's message queue (ZMQ), used by both
       Blockbook and back-end configuration template. See note on templates below.

* `backend` – Definition of back-end package, configuration and service.
    * `package_name` – Name of package. See convention note in [build guide](/docs/build.md#on-naming-conventions-and-versioning).
    * `package_revision` – Revision of package. See convention note in [build guide](/docs/build.md#on-naming-conventions-and-versioning).
    * `system_user` – User used to run back-end service. See convention note in [build guide](/docs/build.md#on-naming-conventions-and-versioning).
    * `version` – Upstream version. See convention note in [build guide](/docs/build.md#on-naming-conventions-and-versioning).
    * `binary_url` – URL of back-end archive.
    * `verification_type` – Type of back-end archive verification. Possible values are *gpg*, *gpg-sha256*, *sha256*.
    * `verification_source` – Source of sign/checksum of back-end archive.
    * `extract_command` – Command to extract back-end archive. It is required to extract content of archive to
       *backend* directory.
    * `exclude_files` – List of files from back-end archive to exclude. Some files are not required for server
       deployment, some binaries have unnecessary dependencies, so it is good idea to extract these files from output
       package. Note that paths are relative to *backend* directory where archive is extracted.
    * `exec_command_template` – Template of command to execute back-end node daemon. Every back-end node daemon has its
       service that is managed by systemd. Template is evaluated to *ExecStart* option in *Service* section of
       service unit. See note on templates below.
    * `logrotate_files_template` – Template that define log files rotated by logrotate daemon. See note on templates
       below.
    * `postinst_script_template` – Additional steps in postinst script. See [ZCash definition](/configs/coins/zcash.json)
       for more information.
    * `service_type` – Type of service. Services that daemonize must have *forking* type and write their PID to
       *PIDFile*. Services that don't support daemonization must have *simple* type. See examples above.
    * `service_additional_params_template` – Additional parameters in service unit. See
       [ZCash definition](/configs/coins/zcash.json) for more information.
    * `protect_memory` – Enables *MemoryDenyWriteExecute* option in service unit if *true*.
    * `mainnet` – Set *false* for testnet back-end.
    * `config_file` – Name of template of back-end configuration file. Templates are defined in *build/backend/config*.
       For Bitcoin-like coins it is not necessary to add extra template, most options can be added via
       *additional_params*. For coins that don't require configuration option should be empty (e.g. Ethereum).
    * `additional_params` – Object of extra parameters that are added to back-end configuration file as key=value pairs.
       Exception is *addnode* key that contains list of nodes that is expanded as addnode=item lines.

* `blockbook` – Definition of Blockbook package, configuration and service.
    * `package_name` – Name of package. See convention note in the [build guide](/docs/build.md#on-naming-conventions-and-versioning).
    * `system_user` – User used to run Blockbook service. See convention note in the [build guide](/docs/build.md#on-naming-conventions-and-versioning).
    * `internal_binding_template` – Template for *-internal* parameter. See note on templates below.
    * `public_binding_template` – Template for *-public* parameter. See note on templates below.
    * `explorer_url` – URL of blockchain explorer. Leave empty for internal explorer.
    * `additional_params` – Additional params of exec command (see [Dogecoin definition](/configs/coins/dogecoin.json)).
    * `block_chain` – Configuration of BlockChain type that ensures communication with back-end service. All options
       must be tweaked for each individual coin separately.
        * `parse` – Use binary parser for block decoding if *true* else call verbose back-end RPC method that returns
           JSON. Note that verbose method is slow and not every coin support it. However there are coin implementations
           that don't support binary parsing (e.g. ZCash).
        * `mempool_workers` – Number of workers for BitcoinType mempool.
        * `mempool_sub_workers` – Number of subworkers for BitcoinType mempool.
        * `block_addresses_to_keep` – Number of blocks that are to be kept in blockaddresses column.
        * `additional_params` – Object of coin-specific params.

* `meta` – Common package metadata.
    * `package_maintainer` – Full name of package maintainer.
    * `package_maintainer_email` – E-mail of package maintainer.

### Go template evaluation note

We use *text/template* package to generate package definitions and configuration files. Some options in coin definition
are also templates and are executed inside base template. Use `{{.path}}` syntax to refer values in coin definition,
where *.path* can be for example *.Blockbook.BlockChain.Parse*. Go uses CamelCase notation so references inside templates
as well. Note that dot at the beginning is mandatory. Go template syntax is fully documented
[here](https://godoc.org/text/template).

## Built-in text

Since Blockbook is an open-source project and we don't prevent anybody from running independent instances, it is possible
to alter built-in text that is specific for Trezor. Text fields that could be updated are:

 * [about](/build/text/about) – A note about instance shown on the Application status page and returned by an API.
 * [tos_link](/build/text/tos_link) – A link to Terms of service shown as the footer on the Explorer pages.

Text data are stored as plain text files in *build/text* directory and are embedded to binary during build. A change of
these files is meant for a private purpose and PRs that would update them won't be accepted.
