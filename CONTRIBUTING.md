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

Execute script *contrib/scripts/check-ports.go* that will check mandatory ports and uniquity of registered ports.

Execute script *contrib/scripts/generate-port-registry.go* that will update *docs/ports.md*.

Now you can try generate package definitions as described above in order to check outputs.

#### Add coin implementation

Coin implementation is stored in *bchain/coins* directory. Each coin must implement interfaces *BlockChain* and
*BlockChainParser* (both defined in [bchain/types.go][/bchain/types.go]) and has registered factory function by
*init()* function of package *blockbook/bchain/coins* ([bchain/coins/blockchain.go](/bchain/coins/blockchain.go)).

There are several approaches how to implement coin support in Blockbook, please see examples below.

Bitcoin package *blockbook/bchain/coins/btc* is reference implementation for Bitcoin-like coins. Most of functinality is
same so particular coin should embed it and override just different parts.

Bitcoin uses binary WIRE protocol thus decoding is very fast but require complex parser. Parser translate whole
pubkey-script to databse ID and therefore it is usually possible store transactions without change.

ZCash package *blockbook/bchain/coins/zec* on the other side uses JSON version of RPCs therefore it dosn't require
specialized parser. Only responsibility that parser has is to translate address to database ID and vice versa.

Ethereum package *blockbook/bchain/coins/eth* must have stand alone implementation because Ethereum uses totally
different concept than Bitcoin.

##### BlockChain interface

Type that implements *bchain.BlockChain* interface ensures communication with block chain network. Because
it calls node RPCs it usually has suffix RPC.

Initialization of object is separated into two stages. At first there is called factory method (details described
in next section) and then *bchain.BlockChain.Initialize()* method. Separated initialization method allows you call
inherited methods during initialization. However it is common practice override fields of embedded structure in factory
method.

During initialization, there is usually loaded chain information, registered message queue callback and created mempool
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

For types that embed base struct it is common practise call factory method of embedded type in order to
create & initialize it. It is much more robust than simple struct composition.

For example see [zec/zcashrpc.go](/bchain/coins/zec/zcashrpc.go).

##### BlockChainParser interface

Type that implements *bchain.BlockChainParser* interface ensures parsing and conversions of block chain data. It is
initialized by *bchain.BlockChain* during initialization.

There are several groups of methods defined in *bchian.BlockChainParser*:

* *GetAddrIDFromVout* and *GetAddrIDFromAddress* – Convert transaction addresses to *[]byte* ID that is used as database ID.
  Most of coins use pubkey-script as ID.
* *AddressToOutputScript* and *OutputScriptToAddresses*  – Convert address to output script and vice versa. Note that
  *btc.BitcoinParser* uses pointer to function *OutputScriptToAddressesFunc* that is called from *OutputScriptToAddress*
  method in order to rewrite implementation by types embdedding it.
* *PackTxid* and *UnpackTxid* – Packs txid to store in database and vice versa.
* *ParseTx* and *ParseTxFromJson* – Parse transaction from binary data or JSON and return *bchain.Tx.
* PackTx* and *UnpackTx* – Pack transaction to binary data to store in database and vice versa.
* *ParseBlock* – Parse block from binary data and return *bchain.Block*.

Base type of parsers is *bchain.BaseParser*. It implements method *ParseTxFromJson* that would be same for all
Bitcoin-like coins. Also implements *PackTx* and *UnpackTx* that pack and unpack transactions using protobuf. Note
that Bitcoin store transactions in hex format.

*bchain.BaseParser* stores pointer to function *bchain.AddressFactoryFunc* that is responsible for making human readable
address representation. See [*bch.BCashParser*](/bchain/coins/bch/bcashparser.go) for example of implementation that uses
different approach for address representation than Bitcoin.

#### Add tests

Add unit tests and integration tests. Tests are described [here](/docs/testing.md).

#### Deploy public server

Deploy Blockbook server on public IP addres. Blockbook maintainers will check implementation before merging.
