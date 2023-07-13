# Blockbook API

**Blockbook** provides REST and websocket API to the indexed blockchain.

## API V2

API V2 is the current version of API. It can be used with all coin types that Blockbook supports. API V2 can be accessed using REST and websocket interface.

Common principles used in API V2:

- all crypto amounts are transferred as strings, in the lowest denomination (satoshis, wei, ...), without decimal point
- empty fields are omitted. Empty field is a string of value _null_ or _""_, a number of value _0_, an object of value _null_ or an array without elements. The reason for this is that the interface serves many different coins which use only subset of the fields. Sometimes this principle can lead to slightly confusing results, for example when transaction version is 0, the field _version_ is omitted.

### REST API

The following methods are supported:

- [Status](#status)
- [Get block hash](#get-block-hash)
- [Get transaction](#get-transaction)
- [Get transaction specific](#get-transaction-specific)
- [Get address](#get-address)
- [Get xpub](#get-xpub)
- [Get utxo](#get-utxo)
- [Get block](#get-block)
- [Send transaction](#send-transaction)
- [Tickers list](#tickers-list)
- [Tickers](#tickers)
- [Balance history](#balance-history)

#### Status page

Status page returns current status of Blockbook and connected backend.

```
GET /api
```

Response:

```javascript
{
  "blockbook": {
    "coin": "Bitcoin",
    "host": "blockbook",
    "version": "0.4.0",
    "gitCommit": "3d9ad91",
    "buildTime": "2019-05-17T14:34:00+00:00",
    "syncMode": true,
    "initialSync": false,
    "inSync": true,
    "bestHeight": 577261,
    "lastBlockTime": "2019-05-22T18:03:33.547762973+02:00",
    "inSyncMempool": true,
    "lastMempoolTime": "2019-05-22T18:10:10.27929383+02:00",
    "mempoolSize": 17348,
    "decimals": 8,
    "dbSize": 191887866502,
    "about": "Blockbook - blockchain indexer for Trezor wallet https://trezor.io/. Do not use for any other purpose."
  },
  "backend": {
    "chain": "main",
    "blocks": 577261,
    "headers": 577261,
    "bestBlockHash": "0000000000000000000ca8c902aa58b3118a7f35d093e25a07f17bcacd91cabf",
    "difficulty": "6704632680587.417",
    "sizeOnDisk": 250504188580,
    "version": "180000",
    "subversion": "/Satoshi:0.18.0/",
    "protocolVersion": "70015",
    "timeOffset": 0,
    "warnings": ""
  }
}
```

#### Get block hash

```
GET /api/v2/block-index/<block height>
```

Response:

```javascript
{
  "blockHash": "ed8f3af8c10ca70a136901c6dd3adf037f0aea8a93fbe9e80939214034300f1e"
}
```

_Note: Blockbook always follows the main chain of the backend it is attached to. See notes on **Get Block** below_

#### Get transaction

Get transaction returns "normalized" data about transaction, which has the same general structure for all supported coins. It does not return coin specific fields (for example information about Zcash shielded addresses).

```
GET /api/v2/tx/<txid>
```

Response for Bitcoin-type coins, confirmed transaction:

```javascript
{
  "txid": "9e2bc8fbd40af17a6564831f84aef0cab2046d4bad19e91c09d21bff2c851851",
  "version": 1,
  "vin": [
    {
      "txid": "f124e6999bf67e710b9e8a8ac4dbb08a64aa9c264120cf98793455e36a531615",
      "vout": 2,
      "sequence": 4294967295,
      "n": 0,
      "addresses": [
        "DDhUv8JZGmSxKYV95NLnbRTUKni9cDZD3S"
      ],
      "isAddress": true,
      "value": "55795108999999",
      "hex": "473...2c7ec77bb982"
    }
  ],
  "vout": [
    {
      "value": "55585679000000",
      "n": 0,
      "hex": "76a914feaca9d9fa7120c7c587c00c639bb18d40faadd388ac",
      "addresses": [
        "DUMh1rPrXTrCN2Z9EHsLPg7b78rACHB2h7"
      ],
      "isAddress": true
    },
    {
      "value": "209329999999",
      "n": 1,
      "hex": "76a914ea8984be785868391d92f49c14933f47c152ea0a88ac",
      "addresses": [
        "DSXDQ6rnwLX47WFRnemctoXPHA9pLMxqXn"
      ],
      "isAddress": true
    }
  ],
  "blockHash": "78d1f3de899a10dd2e580704226ebf9508e95e1706f177fc9c31c47f245d2502",
  "blockHeight": 2647927,
  "confirmations": 1,
  "blockTime": 1553088212,
  "size": 234,
  "vsize": 153,
  "value": "55795008999999",
  "valueIn": "55795108999999",
  "fees": "100000000",
  "hex": "0100000...0011000"
}
```

Response for Bitcoin-type coins, unconfirmed transaction (_blockHeight_: -1, _confirmations_: 0, mining estimates _confirmationETABlocks_ and _confirmationETASeconds_):

```javascript
{
  "txid": "cd8ec77174e426070d0a50779232bba7312b712e2c6843d82d963d7076c61366",
  "version": 2,
  "vin": [
    {
      "txid": "47687cc4abb58d815168686465a38113a0608b2568a6d6480129d197e653f6dc",
      "sequence": 4294967295,
      "n": 0,
      "addresses": ["bc1qka0gpenex558g8gpxmpx247mwhw695k6a7yhs4"],
      "isAddress": true,
      "value": "1983687"
    }
  ],
  "vout": [
    {
      "value": "3106",
      "n": 0,
      "hex": "0020d7da4868055fde790a8581637ab81c216e17a3f8a099283da6c4a27419ffa539",
      "addresses": [
        "bc1q6ldys6q9tl08jz59s93h4wquy9hp0glc5zvjs0dxcj38gx0l55uspu8x86"
      ],
      "isAddress": true
    },
    {
      "value": "1979101",
      "n": 1,
      "hex": "0014381be30ca46ddf378ef69ebc4a601bd6ff30b754",
      "addresses": ["bc1q8qd7xr9ydh0n0rhkn67y5cqm6mlnpd65dcyeeg"],
      "isAddress": true
    }
  ],
  "blockHeight": -1,
  "confirmations": 0,
  "confirmationETABlocks": 3,
  "confirmationETASeconds": 2055,
  "blockTime": 1675270935,
  "size": 234,
  "vsize": 153,
  "value": "1982207",
  "valueIn": "1983687",
  "fees": "1480",
  "hex": "020000000001...b18f00000000"
}
```

Response for Ethereum-type coins. Data of the transaction consist of:

- always only one _vin_, only one _vout_
- an array of _tokenTransfers_ (ERC20, ERC721 or ERC1155)
- _ethereumSpecific_ data
  - _type_ (returned only for contract creation - value `1` and destruction value `2`)
  - _status_ (`1` OK, `0` Failure, `-1` pending), potential _error_ message, _gasLimit_, _gasUsed_, _gasPrice_, _nonce_, input _data_
  - parsed input data in the field _parsedData_, if a match with the 4byte directory was found
  - internal transfers (type `0` transfer, type `1` contract creation, type `2` contract destruction)
- _addressAliases_ - maps addresses in the transaction to names from contract or ENS. Only addresses with known names are returned.

```javascript
{
  "txid": "0xa6c8ae1f91918d09cf2bd67bbac4c168849e672fd81316fa1d26bb9b4fc0f790",
  "vin": [
    {
      "n": 0,
      "addresses": ["0xd446089cf19C3D3Eb1743BeF3A852293Fd2C7775"],
      "isAddress": true
    }
  ],
  "vout": [
    {
      "value": "5615959129349132871",
      "n": 0,
      "addresses": ["0xC36442b4a4522E871399CD717aBDD847Ab11FE88"],
      "isAddress": true
    }
  ],
  "blockHash": "0x10ea8cfecda89d6d864c1d919911f819c9febc2b455b48c9918cee3c6cdc4adb",
  "blockHeight": 16529834,
  "confirmations": 3,
  "blockTime": 1675204631,
  "value": "5615959129349132871",
  "fees": "19141662404282012",
  "tokenTransfers": [
    {
      "type": "ERC20",
      "from": "0xd446089cf19C3D3Eb1743BeF3A852293Fd2C7775",
      "to": "0x3B685307C8611AFb2A9E83EBc8743dc20480716E",
      "contract": "0x4E15361FD6b4BB609Fa63C81A2be19d873717870",
      "name": "Fantom Token",
      "symbol": "FTM",
      "decimals": 18,
      "value": "15362368338194882707417"
    },
    {
      "type": "ERC20",
      "from": "0xC36442b4a4522E871399CD717aBDD847Ab11FE88",
      "to": "0x3B685307C8611AFb2A9E83EBc8743dc20480716E",
      "contract": "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
      "name": "Wrapped Ether",
      "symbol": "WETH",
      "decimals": 18,
      "value": "5615959129349132871"
    },
    {
      "type": "ERC721",
      "from": "0x0000000000000000000000000000000000000000",
      "to": "0xd446089cf19C3D3Eb1743BeF3A852293Fd2C7775",
      "contract": "0xC36442b4a4522E871399CD717aBDD847Ab11FE88",
      "name": "Uniswap V3 Positions NFT-V1",
      "symbol": "UNI-V3-POS",
      "decimals": 18,
      "value": "428189"
    }
  ],
  "ethereumSpecific": {
    "status": 1,
    "nonce": 505,
    "gasLimit": 550941,
    "gasUsed": 434686,
    "gasPrice": "44035608242",
    "data": "0xac9650d800000000000000000000",
    "parsedData": {
      "methodId": "0xfa2b068f",
      "name": "Mint",
      "function": "mint(address, uint256, uint32, bytes32[], address)",
      "params": [
        {
          "type": "address",
          "values": ["0xa5fD1Da088598e88ba731B0E29AECF0BC2A31F82"]
        },
        { "type": "uint256", "values": ["688173296"] },
        { "type": "uint32", "values": ["0"] }
      ]
    },
    "internalTransfers": [
      {
        "type": 0,
        "from": "0xC36442b4a4522E871399CD717aBDD847Ab11FE88",
        "to": "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
        "value": "5615959129349132871"
      }
    ]
  },
  "addressAliases": {
    "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2": {
      "Type": "Contract",
      "Alias": "Wrapped Ether"
    },
    "0xC36442b4a4522E871399CD717aBDD847Ab11FE88": {
      "Type": "Contract",
      "Alias": "Uniswap V3 Positions NFT-V1"
    }
  }
}

```

A note about the `blockTime` field:

- for already mined transaction (`confirmations > 0`), the field `blockTime` contains time of the block
- for transactions in mempool (`confirmations == 0`), the field contains time when the running instance of Blockbook was first time notified about the transaction. This time may be different in different instances of Blockbook.

#### Get transaction specific

Returns transaction data in the exact format as returned by backend, including all coin specific fields:

```
GET /api/v2/tx-specific/<txid>
```

Example response:

```javascript
{
  "hex": "040000808...8e6e73cb009",
  "txid": "7a0a0ff6f67bac2a856c7296382b69151949878de6fb0d01a8efa197182b2913",
  "overwintered": true,
  "version": 4,
  "versiongroupid": "892f2085",
  "locktime": 0,
  "expiryheight": 495680,
  "vin": [],
  "vout": [],
  "vjoinsplit": [],
  "valueBalance": 0,
  "vShieldedSpend": [
    {
      "cv": "50258bfa65caa9f42f4448b9194840c7da73afc8159faf7358140bfd0f237962",
      "anchor": "6beb3b64ecb30033a9032e1a65a68899917625d1fdd2540e70f19f3078f5dd9b",
      "nullifier": "08e5717f6606af7c2b01206ff833eaa6383bb49c7451534b2e16d588956fd10a",
      "rk": "36841a9be87a7022445b77f433cdd0355bbed498656ab399aede1e5285e9e4a2",
      "proof": "aecf824dbae8eea863ec6...73878c37391f01df520aa",
      "spendAuthSig": "65b9477cb1ec5da...1178fe402e5702c646945197108339609"
    },
    {
      "cv": "a5aab3721e33d6d6360eabd21cbd07524495f202149abdc3eb30f245d503678c",
      "anchor": "6beb3b64ecb30033a9032e1a65a68899917625d1fdd2540e70f19f3078f5dd9b",
      "nullifier": "60e790d6d0e12e777fb2b18bc97cf42a92b1e47460e1bd0b0ffd294c23232cc9",
      "rk": "2d741695e76351597712b4a04d2a4e108a116f376283d2d104219b86e2930117",
      "proof": "a0c2a6fdcbba966b9894...3a9c3118b76c8e2352d524cbb44c02decaeda7",
      "spendAuthSig": "feea902e01eac9ebd...b43b4af6b607ce5b0b38f708"
    }
  ],
  "vShieldedOutput": [
    {
      "cv": "23db384cde862f20238a1004e57ba18f114acabc7fd2ac029757f82af5bd4cab",
      "cmu": "3ff5a5ff521fabefb5287fef4feb2642d69ead5fe18e6ac717cfd76a8d4088bc",
      "ephemeralKey": "057ff6e059967784fa6ac34ad9ecfd9c0c0aba743b7cd444a65ecc32192d5870",
      "encCiphertext": "a533d3b99b...a0204",
      "outCiphertext": "4baabc15199504b1...c1ad6a",
      "proof": "aa1fb2706cba5...1ec7e81f5deea90d4f57713f3b4fc8d636908235fa378ebf1"
    }
  ],
  "bindingSig": "bc018af8808387...5130bb382ad8e6e73cb009",
  "blockhash": "0000000001c4aa394e796dd1b82e358f114535204f6f5b6cf4ad58dc439c47af",
  "confirmations": 5222,
  "time": 1552301566,
  "blocktime": 1552301566
}
```

#### Get address

Returns balances and transactions of an address. The returned transactions are sorted by block height, newest blocks first.

```
GET /api/v2/address/<address>[?page=<page>&pageSize=<size>&from=<block height>&to=<block height>&details=<basic|tokens|tokenBalances|txids|txs>&contract=<contract address>&secondary=usd]
```

The optional query parameters:

- _page_: specifies page of returned transactions, starting from 1. If out of range, Blockbook returns the closest possible page.
- _pageSize_: number of transactions returned by call (default and maximum 1000)
- _from_, _to_: filter of the returned transactions _from_ block height _to_ block height (default no filter)
- _details_: specifies level of details returned by request (default _txids_)
  - _basic_: return only address balances, without any transactions
  - _tokens_: _basic_ + tokens belonging to the address (applicable only to some coins)
  - _tokenBalances_: _basic_ + tokens with balances + belonging to the address (applicable only to some coins)
  - _txids_: _tokenBalances_ + list of txids, subject to _from_, _to_ filter and paging
  - _txslight_: _tokenBalances_ + list of transaction with limited details (only data from index), subject to _from_, _to_ filter and paging
  - _txs_: _tokenBalances_ + list of transaction with details, subject to _from_, _to_ filter and paging
- _contract_: return only transactions which affect specified contract (applicable only to coins which support contracts)
- _secondary_: specifies secondary (fiat) currency in which the token and total balances are returned in addition to crypto values

Example response for bitcoin type coin, _details_ set to _txids_:

```javascript
{
  "page": 1,
  "totalPages": 1,
  "itemsOnPage": 1000,
  "address": "D5Z7XrtJNg7hAtznSDMXvfiFmMYphwuWz7",
  "balance": "2432468097999991",
  "totalReceived": "3992283916999979",
  "totalSent": "1559815818999988",
  "unconfirmedBalance": "0",
  "unconfirmedTxs": 0,
  "txs": 3,
  "txids": [
    "461dd46d5d6f56d765f82e60e6bf0727a3a1d1cb8c4144373d805b152a21d308",
    "bdb5b47603c5d174eae3384c368068c8e9d2183b398ed0e31d125defa4447a10",
    "5c1d2686d70d82bd8e84b5d3dc4bd0e8485e28cdc865336db6a5e40b2098277d"
  ]
}
```

Example response for ethereum type coin, _details_ set to _tokenBalances_ and _secondary_ set to _usd_. The _baseValue_ is value of the token in the base currency (ETH), _secondaryValue_ is value of the token in specified _secondary_ currency:

```javascript
{
  "address": "0x2df3951b2037bA620C20Ed0B73CCF45Ea473e83B",
  "balance": "21004631949601199",
  "unconfirmedBalance": "0",
  "unconfirmedTxs": 0,
  "txs": 5,
  "nonTokenTxs": 3,
  "nonce": "1",
  "tokens": [
    {
      "type": "ERC20",
      "name": "Tether USD",
      "contract": "0xdAC17F958D2ee523a2206206994597C13D831ec7",
      "transfers": 3,
      "symbol": "USDT",
      "decimals": 6,
      "balance": "4913000000",
      "baseValue": 3.104622978658881,
      "secondaryValue": 4914.214559070491
    }
  ],
  "secondaryValue": 33.247601671503574,
  "tokensBaseValue": 3.104622978658881,
  "tokensSecondaryValue": 4914.214559070491,
  "totalBaseValue": 3.125627610608482,
  "totalSecondaryValue": 4947.462160741995
}

```

#### Get xpub

Returns balances and transactions of an xpub or output descriptor, applicable only for Bitcoin-type coins.

Blockbook supports BIP44, BIP49, BIP84 and BIP86 (Taproot) derivation schemes, using either xpubs or output descriptors (see https://github.com/bitcoin/bitcoin/blob/master/doc/descriptors.md)

- Xpubs

  Blockbook expects xpub at level 3 derivation path, i.e. _m/purpose'/coin_type'/account'/_. Blockbook completes the _change/address_index_ part of the path when deriving addresses.
  The BIP version is determined by the prefix of the xpub. The prefixes for each coin are defined by fields `xpub_magic`, `xpub_magic_segwit_p2sh`, `xpub_magic_segwit_native` in the [trezor-common](https://github.com/trezor/trezor-common/tree/master/defs/bitcoin) library. If the prefix is not recognized, Blockbook defaults to BIP44 derivation scheme.

- Output descriptors

  Output descriptors are in the form `<type>([<path>]<xpub>[/<change>/*])[#checksum]`, for example `pkh([5c9e228d/44'/0'/0']xpub6BgBgses...Mj92pReUsQ/<0;1>/*)#abcd`

  Parameters `type` and `xpub` are mandatory, the rest is optional

  Blockbook supports a limited set of `type`s:

  - BIP44: `pkh(xpub)`
  - BIP49: `sh(wpkh(xpub))`
  - BIP84: `wpkh(xpub)`
  - BIP86 (Taproot single key): `tr(xpub)`

  Parameter `change` can be a single number or a list of change indexes, specified either in the format `<index1;index2;...>` or `{index1,index2,...}`. If the parameter `change` is not specified, Blockbook defaults to `<0;1>`.

The returned transactions are sorted by block height, newest blocks first.

```
GET /api/v2/xpub/<xpub|descriptor>[?page=<page>&pageSize=<size>&from=<block height>&to=<block height>&details=<basic|tokens|tokenBalances|txids|txs>&tokens=<nonzero|used|derived>&secondary=eur]
```

The optional query parameters:

- _page_: specifies page of returned transactions, starting from 1. If out of range, Blockbook returns the closest possible page.
- _pageSize_: number of transactions returned by call (default and maximum 1000)
- _from_, _to_: filter of the returned transactions _from_ block height _to_ block height (default no filter)
- _details_: specifies level of details returned by request (default _txids_)
  - _basic_: return only xpub balances, without any derived addresses and transactions
  - _tokens_: _basic_ + tokens (addresses) derived from the xpub, subject to _tokens_ parameter
  - _tokenBalances_: _basic_ + tokens (addresses) derived from the xpub with balances, subject to _tokens_ parameter
  - _txids_: _tokenBalances_ + list of txids, subject to _from_, _to_ filter and paging
  - _txs_: _tokenBalances_ + list of transaction with details, subject to _from_, _to_ filter and paging
- _tokens_: specifies what tokens (xpub addresses) are returned by the request (default _nonzero_)
  - _nonzero_: return only addresses with nonzero balance
  - _used_: return addresses with at least one transaction
  - _derived_: return all derived addresses
- _secondary_: specifies secondary (fiat) currency in which the balances are returned in addition to crypto values

Response:

```javascript
{
  "page": 1,
  "totalPages": 1,
  "itemsOnPage": 1000,
  "address": "dgub8sbe5Mi8LA4dXB9zPfLZW8arm...9Vjp2HHx91xdDEmWYpmD49fpoUYF",
  "balance": "90000000",
  "totalReceived": "3093381250",
  "totalSent": "3083381250",
  "unconfirmedBalance": "0",
  "unconfirmedTxs": 0,
  "txs": 5,
  "txids": [
    "383ccb5da16fccad294e24a2ef77bdee5810573bb1b252d8b2af4f0ac8c4e04c",
    "75fb93d47969ac92112628e39148ad22323e96f0004c18f8c75938cffb6c1798",
    "e8cd84f204b4a42b98e535e72f461dd9832aa081458720b0a38db5856a884876",
    "57833d50969208091bd6c950599a1b5cf9d66d992ae8a8d3560fb943b98ebb23",
    "9cfd6295f20e74ddca6dd816c8eb71a91e4da70fe396aca6f8ce09dc2947839f",
  ],
  "usedTokens": 2,
  "tokens": [
    {
      "type": "XPUBAddress",
      "name": "DUCd1B3YBiXL5By15yXgSLZtEkvwsgEdqS",
      "path": "m/44'/3'/0'/0/0",
      "transfers": 3,
      "decimals": 8,
      "balance": "90000000",
      "totalReceived": "2903986975",
      "totalSent": "2803986975"
    },
    {
      "type": "XPUBAddress",
      "name": "DKu2a8Wo6zC2dmBBYXwUG3fxWDHbKnNiPj",
      "path": "m/44'/3'/0'/1/0",
      "transfers": 2,
      "decimals": 8,
      "balance": "0",
      "totalReceived": "279394275",
      "totalSent": "279394275"
    }
  ],
  "secondaryValue": 21195.47633568
}
```

Note: _usedTokens_ always returns total number of **used** addresses of xpub.

#### Get utxo

Returns array of unspent transaction outputs of address or xpub, applicable only for Bitcoin-type coins. By default, the list contains both confirmed and unconfirmed transactions. The query parameter _confirmed=true_ disables return of unconfirmed transactions. The returned utxos are sorted by block height, newest blocks first. For xpubs or output descriptors, the response also contains address and derivation path of the utxo.

Unconfirmed utxos do not have field _height_, the field _confirmations_ has value _0_ and may contain field _lockTime_, if not zero.

Coinbase utxos have field _coinbase_ set to true, however due to performance reasons only up to minimum coinbase confirmations limit (100). After this limit, utxos are not detected as coinbase.

```
GET /api/v2/utxo/<address|xpub|descriptor>[?confirmed=true]
```

Response:

```javascript
[
  {
    txid: "13d26cd939bf5d155b1c60054e02d9c9b832a85e6ec4f2411be44b6b5a2842e9",
    vout: 0,
    value: "1422303206539",
    confirmations: 0,
    lockTime: 2648100,
  },
  {
    txid: "a79e396a32e10856c97b95f43da7e9d2b9a11d446f7638dbd75e5e7603128cac",
    vout: 1,
    value: "39748685",
    height: 2648043,
    confirmations: 47,
    coinbase: true,
  },
  {
    txid: "de4f379fdc3ea9be063e60340461a014f372a018d70c3db35701654e7066b3ef",
    vout: 0,
    value: "122492339065",
    height: 2646043,
    confirmations: 2047,
  },
  {
    txid: "9e8eb9b3d2e8e4b5d6af4c43a9196dfc55a05945c8675904d8c61f404ea7b1e9",
    vout: 0,
    value: "142771322208",
    height: 2644885,
    confirmations: 3205,
  },
];
```

#### Get block

Returns information about block with transactions, subject to paging.

```
GET /api/v2/block/<block height|block hash>
```

Response:

```javascript
{
  "page": 1,
  "totalPages": 1,
  "itemsOnPage": 1000,
  "hash": "760f8ed32894ccce9c1ea11c8a019cadaa82bcb434b25c30102dd7e43f326217",
  "previousBlockHash": "786a1f9f38493d32fd9f9c104d748490a070bc74a83809103bcadd93ae98288f",
  "nextBlockHash": "151615691b209de41dda4798a07e62db8429488554077552ccb1c4f8c7e9f57a",
  "height": 2648059,
  "confirmations": 47,
  "size": 951,
  "time": 1553096617,
  "version": 6422787,
  "merkleRoot": "6783f6083788c4f69b8af23bd2e4a194cf36ac34d590dfd97e510fe7aebc72c8",
  "nonce": "0",
  "bits": "1a063f3b",
  "difficulty": "2685605.260733312",
  "txCount": 2,
  "txs": [
    {
      "txid": "2b9fc57aaa8d01975631a703b0fc3f11d70671953fc769533b8078a04d029bf9",
      "vin": [
        {
          "n": 0,
          "value": "0"
        }
      ],
      "vout": [
        {
          "value": "1000100000000",
          "n": 0,
          "addresses": [
            "D6ravJL6Fgxtgp8k2XZZt1QfUmwwGuLwQJ"
          ],
          "isAddress": true
        }
      ],
      "blockHash": "760f8ed32894ccce9c1ea11c8a019cadaa82bcb434b25c30102dd7e43f326217",
      "blockHeight": 2648059,
      "confirmations": 47,
      "blockTime": 1553096617,
      "value": "1000100000000",
      "valueIn": "0",
      "fees": "0"
    },
    {
      "txid": "d7ce10ecf9819801ecd6ee045cbb33436eef36a7db138206494bacedfd2832cf",
      "vin": [
        {
          "n": 0,
          "addresses": [
            "9sLa1AKzjWuNTe1CkLh5GDYyRP9enb1Spp"
          ],
          "isAddress": true,
          "value": "1277595845202"
        }
      ],
      "vout": [
        {
          "value": "9900000000",
          "n": 0,
          "addresses": [
            "DMnjrbcCEoeyvr7GEn8DS4ZXQjwq7E2zQU"
          ],
          "isAddress": true
        },
        {
          "value": "1267595845202",
          "n": 1,
          "spent": true,
          "addresses": [
            "9sLa1AKzjWuNTe1CkLh5GDYyRP9enb1Spp"
          ],
          "isAddress": true
        }
      ],
      "blockHash": "760f8ed32894ccce9c1ea11c8a019cadaa82bcb434b25c30102dd7e43f326217",
      "blockHeight": 2648059,
      "confirmations": 47,
      "blockTime": 1553096617,
      "value": "1277495845202",
      "valueIn": "1277595845202",
      "fees": "100000000"
    }
  ]
}
```

_Note: Blockbook always follows the main chain of the backend it is attached to. If there is a rollback-reorg in the backend, Blockbook will also do rollback. When you ask for block by height, you will always get the main chain block. If you ask for block by hash, you may get the block from another fork but it is not guaranteed (backend may not keep it)_

#### Send transaction

Sends new transaction to backend.

```
GET /api/v2/sendtx/<hex tx data>
POST /api/v2/sendtx/ (hex tx data in request body)  NB: the '/' symbol at the end is mandatory.
```

Response:

```javascript
{
  "result": "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25"
}
```

or in case of error

```javascript
{
  "error": {
    "message": "error message"
  }
}
```

#### Tickers list

Returns a list of available currency rate tickers (secondary currencies) for the specified date, along with an actual data timestamp.

```
GET /api/v2/tickers-list/?timestamp=<timestamp>
```

The query parameters:

- _timestamp_: specifies a Unix timestamp to return available tickers for.

Example response:

```javascript
{
  "ts":1574346615,
  "available_currencies": [
    "eur",
    "usd"
  ]
}
```

#### Tickers

Returns currency rate for the specified currency and date. If the currency is not available for that specific timestamp, the next closest rate will be returned.
All responses contain an actual rate timestamp.

```
GET /api/v2/tickers/[?currency=<currency>&timestamp=<timestamp>]
```

The optional query parameters:

- _currency_: specifies a currency of returned rate ("usd", "eur", "eth"...). If not specified, all available currencies will be returned.
- _timestamp_: a Unix timestamp that specifies a date to return currency rates for. If not specified, the last available rate will be returned.

Example response (no parameters):

```javascript
{
  "ts": 1574346615,
  "rates": {
    "eur": 7134.1,
    "usd": 7914.5
    }
}
```

Example response (currency=usd):

```javascript
{
  "ts": 1574346615,
  "rates": {
    "usd": 7914.5
  }
}
```

Example error response (e.g. rate unavailable, incorrect currency...):

```javascript
{
  "ts":7980386400,
  "rates": {
    "usd": -1
  }
}
```

#### Balance history

Returns a balance history for the specified XPUB or address.

```
GET /api/v2/balancehistory/<XPUB | address>?from=<dateFrom>&to=<dateTo>[&fiatcurrency=<currency>&groupBy=<groupBySeconds>]
```

Query parameters:

- _from_: specifies a start date as a Unix timestamp
- _to_: specifies an end date as a Unix timestamp

The optional query parameters:

- _fiatcurrency_: if specified, the response will contain secondary (fiat) rate at the time of transaction. If not, all available currencies will be returned.
- _groupBy_: an interval in seconds, to group results by. Default is 3600 seconds.

Example response (_fiatcurrency_ not specified):

```javascript
[
  {
    "time": 1578391200,
    "txs": 5,
    "received": "5000000",
    "sent": "0",
    "sentToSelf":"100000",
    "rates": {
      "usd": 7855.9,
      "eur": 6838.13,
      ...
    }
  },
  {
    "time": 1578488400,
    "txs": 1,
    "received": "0",
    "sent": "5000000",
    "sentToSelf":"0",
    "rates": {
      "usd": 8283.11,
      "eur": 7464.45,
      ...
    }
  }
]
```

Example response (fiatcurrency=usd):

```javascript
[
  {
    time: 1578391200,
    txs: 5,
    received: "5000000",
    sent: "0",
    sentToSelf: "0",
    rates: {
      usd: 7855.9,
    },
  },
  {
    time: 1578488400,
    txs: 1,
    received: "0",
    sent: "5000000",
    sentToSelf: "0",
    rates: {
      usd: 8283.11,
    },
  },
];
```

Example response (fiatcurrency=usd&groupBy=172800):

```javascript
[
  {
    time: 1578355200,
    txs: 6,
    received: "5000000",
    sent: "5000000",
    sentToSelf: "0",
    rates: {
      usd: 7734.45,
    },
  },
];
```

The value of `sentToSelf` is the amount sent from the same address to the same address or within addresses of xpub.

### Websocket API

Websocket interface is provided at `/websocket/`. The interface can be explored using Blockbook Websocket Test Page found at `/test-websocket.html`.

The websocket interface provides the following requests:

- getInfo
- getBlockHash
- getAccountInfo
- getAccountUtxo
- getTransaction
- getTransactionSpecific
- getBalanceHistory
- getCurrentFiatRates
- getFiatRatesTickersList
- getFiatRatesForTimestamps
- estimateFee
- sendTransaction
- ping

The client can subscribe to the following events:

- `subscribeNewBlock` - new block added to blockchain
- `subscribeNewTransaction` - new transaction added to blockchain (all addresses)
- `subscribeAddresses` - new transaction for a given address (list of addresses) added to mempool
- `subscribeFiatRates` - new currency rate ticker

There can be always only one subscription of given event per connection, i.e. new list of addresses replaces previous list of addresses.

The subscribeNewTransaction event is not enabled by default. To enable support, blockbook must be run with the `-enablesubnewtx` flag.

_Note: If there is reorg on the backend (blockchain), you will get a new block hash with the same or even smaller height if the reorg is deeper_

Websocket communication format

```javascript
{
  "id":"1", //an id to help to identify the response
  "method":"<The method that you would like to call>",
  "params":<The params (same as in the API call>
}
```

Example for subscribing to an address (or multiple addresses)

```javascript
{
  "id":"1",
  "method":"subscribeAddresses",
  "params":{
    "addresses":["mnYYiDCb2JZXnqEeXta1nkt5oCVe2RVhJj", "tb1qp0we5epypgj4acd2c4au58045ruud2pd6heuee"]
   }
}
```

## Legacy API V1

The legacy API is a compatible subset of API provided by **Bitcore Insight**. It is supported only Bitcoin-type coins. The details of the REST/socket.io requests can be found in the Insight's documentation.

### REST API

```
GET /api/v1/block-index/<block height>
GET /api/v1/tx/<txid>
GET /api/v1/address/<address>
GET /api/v1/utxo/<address>
GET /api/v1/block/<block height | block hash>
GET /api/v1/estimatefee/<number of blocks>
GET /api/v1/sendtx/<hex tx data>
POST /api/v1/sendtx/ (hex tx data in request body)
```

### Socket.io API

Socket.io interface is provided at `/socket.io/`. The interface also can be explored using Blockbook Socket.io Test Page found at `/test-socketio.html`.

The legacy API is provided as is and will not be further developed.

The legacy API is currently (as of Blockbook v0.4.0) also accessible without the _/v1/_ prefix, however in the future versions the version less access will be removed.
