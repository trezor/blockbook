# Blockbook Contributor Guide

Blockbook is a back-end service for the Trezor wallet. Although it is open source, the design and development of the core packages
is done by the Trezor developers to keep Blockbook compatible with Trezor.

Bug fixes and support for new coins are welcome. **Please take note that pull requests that are not fixes and that change base
packages or another coin code will not be accepted.** If you need a change in the existing core code, please file
an issue and discuss your request with the Blockbook maintainers.

## Development environment

Instructions to set up your development environment and build Blockbook are described in a separate
[document](/docs/build.md).

## How can I contribute?

### Reporting bugs

A great way to contribute to the project is to send a detailed report when you encounter a problem. We always appreciate
a well-written and thorough bug report, and we'll be grateful for it!

Check that [our issue database](https://github.com/trezor/blockbook/issues) doesn't already include that problem or
suggestion before submitting an issue. If you find a match, you can use the "subscribe" button to get notified on
updates. Do not leave random "+1" or "I have this too" comments, as they only clutter the discussion, and don't help
resolving it. However, if you have ways to reproduce the issue or have additional information that may help resolving
the issue, please leave a comment.

Include information about the Blockbook instance, which is shown at the Blockbook status page or returned by API call. For example execute `curl -k https://<server name>:<public port>/api` to get JSON containing details about Blockbook and Backend installation.  Ports are listed in the [port registry](/docs/ports.md). 

Also include the steps required to reproduce the problem if possible and applicable. This information will help us
review and fix your issue faster. When sending lengthy log-files, consider posting them as a gist
(https://gist.github.com).

### Adding coin support

> **Important notice**: Although we are happy for support of new coins, we do not have enough capacity to run them all
> on our infrastructure. We run Blockbook instances only for selected number of coins. If you want to have Blockbook
> instance for your coin, you will have to deploy it to your own server.

Trezor harware wallet supports over 500 coins, see https://trezor.io/coins/. You are free to add support for any of
them to Blockbook. Currently implemented coins are listed [here](/docs/ports.md).

You should follow the steps below to get smooth merge of your PR.

#### Add coin definition

Coin definitions are stored in JSON files in *configs/coins* directory. They are the single source of Blockbook
configuration, Blockbook and back-end package definition and build metadata. Since Blockbook supports only single
coin index per running instance, every coin (including testnet) must have single definition file.

All options of coin definition are described in [config.md](/docs/config.md).

Because most of coins are fork of Bitcoin and they have similar way to install and configure their daemon, we use
templates to generate package definition and configuration files during build process. Similarly, there are templates for Blockbook
package. Templates are filled with data from coin definition. Although normally all package definitions are generated automatically
during the build process, sometimes there is a reason to check what was generated. You can create them by calling
`go run build/templates/generate.go coin`, where *coin* is name of definition file without .json extension. Files are
generated to *build/pkg-defs* directory.

Good examples of coin configuration are
[*configs/coins/bitcoin.json*](configs/coins/bitcoin.json) and
[*configs/coins/ethereum.json*](configs/coins/ethereum.json) for Bitcoin type coins and Ethereum type coins, respectively.

Usually you have to update only a few options that differ from the Bitcoin definition. At first there is base information
about coin in section *coin* – name, alias etc. Then update port information in *port* section. We keep port series as
listed in [the port registry](/docs/ports.md). Select next port numbers in the series. Port numbers must be unique across all
port definitions.

In the section *backend* update information how to build and configure back-end service. When back-end package is built,
build process downloads installation archive, verifies and extracts it. How it is done is described in
[build guide](/docs/build.md#on-back-end-building). Naming conventions and versioning are described
also in [build guide](/docs/build.md#on-naming-conventions-and-versioning). You have to update *package_name*,
*package_revision*, *system_user*, *version*, *binary_url*, *verification_type*, *verification_source*,
*extract_command* and *exclude_files*. Also update information whether service runs mainnet or testnet network in
*mainnet* option.

In the section *blockbook* update information how to build and configure Blockbook service. Usually they are only
*package_name*, *system_user* and *explorer_url* options. Naming conventions are described
[here](/docs/build.md#on-naming-conventions-and-versioning).

Update *package_maintainer* and *package_maintainer_email* options in the section *meta*.

Execute script *go run contrib/scripts/check-and-generate-port-registry.go -w* that checks mandatory ports and
uniqueness of ports and updates registry of ports *docs/ports.md*.

Now you can try to generate package definitions as described above in order to check outputs.

#### Add coin implementation

Coin implementation is stored in *bchain/coins* directory. Each coin must implement interfaces *BlockChain* and
*BlockChainParser* (both defined in [bchain/types.go][/bchain/types.go]) and has registered factory function by
*init()* function of package *blockbook/bchain/coins* ([bchain/coins/blockchain.go](/bchain/coins/blockchain.go)).

There are several approaches how to implement coin support in Blockbook, please see examples below.

Bitcoin package *blockbook/bchain/coins/btc* is a reference implementation for Bitcoin-like coins. Most of the functionality
is usually the same so particular coin should embed it and override just different parts.

Bitcoin uses binary WIRE protocol thus decoding is very fast but require complex parser. Parser translate whole
pubkey-script to database ID and therefore it is usually possible store transactions without change.

ZCash package *blockbook/bchain/coins/zec* on the other side uses JSON version of RPCs therefore it doesn't require
specialized parser. Only responsibility that parser has is to translate address to Address Descriptor (used as 
address ID in the database) and vice versa.

Ethereum package *blockbook/bchain/coins/eth* has own stand alone implementation because Ethereum uses totally
different concept than Bitcoin.

##### BlockChain interface

Type that implements *bchain.BlockChain* interface ensures communication with the block chain network. Because
it calls node RPCs, it usually has suffix RPC.

Initialization of object is separated into two stages. At first there is called factory method (details described
in the next section) and then *bchain.BlockChain.Initialize()* method. Separated initialization method allows you call
inherited methods during initialization. However it is common practice override fields of embedded structure in factory
method.

Initialization routine usually loads chain information, registers message queue callback and creates mempool
and parser objects.

BitcoinRPC uses *btc.RPCMarshaller* ([btc/codec.go](/bchain/coins/btc/codec.go)) in order to distinguish API version of
Bitcoin RPC. Current API (*btc.JSONMarshalerV2*) uses JSON object with method arguments. Older API (*btc.JSONMarshalerV1*)
uses JSON array with method arguments and some arguments are defined differently (e.g. bool vs int).

For example see [zec/zcashrpc.go](/bchain/coins/zec/zcashrpc.go).

##### BlockChain factory function

Factory function must be *coins.blockChainFactory* type ([coins/blockchain.go](/bchain/coins/blockchain.go)). It gets
configuration as JSON object and handler function for PUSH notifications. All factory functions have registered by
*init()* function of package *blockbook/bchain/coins* ([coins/blockchain.go](/bchain/coins/blockchain.go)). Coin name
must correspond to *coin.name* in coin definition file (see above).

Configuration passed to factory method is coin specific. For types that embed *btc.BitcoinRPC,* configuration must
contain at least fields referred in *btc.Configuration* ([btc/bitcoinrpc.go](/bchain/coins/btc/bitcoinrpc.go)).

For types that embed base struct it is common practice to call factory method of the embedded type in order to
create & initialize it. It is much more robust than simple struct composition.

For example see [zec/zcashrpc.go](/bchain/coins/zec/zcashrpc.go).

##### BlockChainParser interface

Type that implements *bchain.BlockChainParser* interface ensures parsing and conversions of block chain data. It is
initialized by *bchain.BlockChain* during initialization.

There are several groups of methods defined in *bchain.BlockChainParser*:

* *GetAddrDescFromVout* and *GetAddrDescFromAddress* – Convert transaction addresses to *Address Descriptor* that is used as database ID.
  Most of coins use output script as *Address Descriptor*.
* *GetAddressesFromAddrDesc* and *GetScriptFromAddrDesc*  – Convert *Address Descriptor* to addresses and output script. Note that
  *btc.BitcoinParser* uses pointer to function *OutputScriptToAddressesFunc* that is called from *GetAddressesFromAddrDesc*
  method in order to rewrite implementation by types embedding it.
* *PackTxid* and *UnpackTxid* – Packs txid to store in database and vice versa.
* *ParseTx* and *ParseTxFromJson* – Parse transaction from binary data or JSON and return *bchain.Tx*.
* *PackTx* and *UnpackTx* – Pack transaction to binary data to store in database and vice versa.
* *ParseBlock* – Parse block from binary data and return *bchain.Block*.

Base type of parsers is *bchain.BaseParser*. It implements method *ParseTxFromJson* that should be the same for all
Bitcoin-like coins. Also implements *PackTx* and *UnpackTx* that pack and unpack transactions using protobuf. Note
that Bitcoin stores transactions in more compact binary format.

*bchain.BaseParser* stores pointer to function *bchain.AddressFactoryFunc* that is responsible for making human readable
address representation. See [*bch.bcashparser*](/bchain/coins/bch/bcashparser.go) for example of implementation that uses
different approach for address representation than Bitcoin.

#### Add tests

Add unit tests and integration tests. **Pull requests without passing tests will not be accepted**. 
How to implement tests is described [here](/docs/testing.md).

#### Deploy public server

Deploy Blockbook server on public IP address. Blockbook maintainers will check your implementation before merging.
