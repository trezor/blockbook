# Blockbook v0.4.0 Changelog

## API:

- Blockbook V1 endpoints are exclusively available for Bitcoin-based 
blockchains, and only supported by Bitcoin Insight. 
[(source)](https://github.com/trezor/blockbook/commit/c1256d22e9e34844acfbae5f094365e74da7dc99#diff-9eddf4dc51bf6a0125ca7fb094468ad284112270385c41cebce4f8f0a29620abR902)

### Xpubs:
```
GET 
/api/v2/xpub/<xpub|descriptor>[?page=<page>&pageSize=<size>&from=<block 
height>&to=<block 
height>&details=<basic|tokens|tokenBalances|txids|txs>&tokens=<nonzero|used|derived>&secondary=eur]
```

- Xpub endpoint now supports specifying secondary fiat currency in which 
the crypto values are converted to. 
[(source)](https://github.com/trezor/blockbook/commit/c1256d22e9e34844acfbae5f094365e74da7dc99#diff-9eddf4dc51bf6a0125ca7fb094468ad284112270385c41cebce4f8f0a29620abR433), 
[(example)](https://github.com/trezor/blockbook/commit/c1256d22e9e34844acfbae5f094365e74da7dc99#diff-9eddf4dc51bf6a0125ca7fb094468ad284112270385c41cebce4f8f0a29620abR371-R404)

### Transactions:
```
GET /api/v2/tx/<txid>
```
#### Bitcoin Transaction:

- **vsize, size and weight** are included - 
[(example)](https://btc1.trezor.io/api/v2/tx/18945c8c8ff2016617a2bac644971ab21d97c51986f2516b31e640616c4d4862)

- unconfirmed transaction (**blockHeight: -1, confirmations: 0, mining 
estimates confirmationETABlocks and confirmationETASeconds**) are 
included. 
[(source)](https://github.com/trezor/blockbook/commit/b227dfedcbde37219a4cb16500529513cc48e1e8#diff-9eddf4dc51bf6a0125ca7fb094468ad284112270385c41cebce4f8f0a29620abR188-R191)

#### Ethereum Transaction:
Response for Ethereum-type coins - 
[(example)](0x92524e7a1164841639951ee3d32d8072e77e0283f2f42a07c5bb8bae6358f0b4). 
Data of the transaction consist of:

- always only one _vin_, only one _vout_
- an array of _tokenTransfers_ (ERC20, ERC721 or ERC1155)
- _ethereumSpecific_ data
  - _type_ (returned only for contract creation - value `1` and 
destruction value `2`)
  - _status_ (`1` OK, `0` Failure, `-1` pending), potential _error_ 
message, _gasLimit_, _gasUsed_, _gasPrice_, _nonce_, input _data_
  - parsed input data in the field _parsedData_, if a match with the 4byte 
directory was found
  - internal transfers (type `0` transfer, type `1` contract creation, 
type `2` contract destruction)
- _addressAliases_ - maps addresses in the transaction to names from 
contract or ENS. Only addresses with known names are returned.

---
## Database:

- In case of the Ethereum type coins, the database is not compatible with 
previous the versions. The database must be recreated by inital 
synchronization with the backend. To process the internal transactions, 
the backend must run in archive mode and the synchronization is slow, can 
take several weeks.
For the Bitcoin type coins, the database upgrades automatically, no action 
is necessary. 
[(source)](https://github.com/trezor/blockbook/releases/tag/v0.4.0)


### Existing Columns:

- default: data format version changed to 6. 
[(source)](https://github.com/trezor/blockbook/commit/c1256d22e9e34844acfbae5f094365e74da7dc99#diff-96484041aa460b98eecdb7c9dbab36e8eb8b37a645415a05590e562f31efc0abR28)

- fiatRates: Stored daily fiat rates, one day as one entry. 
[(source)](https://github.com/trezor/blockbook/commit/c1256d22e9e34844acfbae5f094365e74da7dc99#diff-96484041aa460b98eecdb7c9dbab36e8eb8b37a645415a05590e562f31efc0abR158)



### New Columns:

- contracts: Added contract address descriptor (addrDesc) mapping to 
provide information about the contract including name, symbol, type 
(ERC20, ERC721, or ERC1155), decimals, created and destructed block 
height. 
[(source)](https://github.com/trezor/blockbook/commit/c1256d22e9e34844acfbae5f094365e74da7dc99#diff-96484041aa460b98eecdb7c9dbab36e8eb8b37a645415a05590e562f31efc0abR170)

- functionSignatures: Added a database for four byte signatures downloaded 
from https://www.4byte.directory/

- blockInternalDataErrors: Added a storage for errors encountered while 
fetching internal data from the backend for Ethereum type coins, to allow 
for retries

- addressAliases: Maps Ethereum addresses to their corresponding Ethereum 
Name Service (ENS) names. 
[(source)](https://github.com/trezor/blockbook/commit/c1256d22e9e34844acfbae5f094365e74da7dc99#diff-96484041aa460b98eecdb7c9dbab36e8eb8b37a645415a05590e562f31efc0abR195)

---
## Environment:
Debian recommended version changed to 11. 
[(source)](https://github.com/trezor/blockbook/commit/1a476e58f07cb32ffecc41392b339428f7075feb#diff-43453f510556d352276e897e137cb103b3bbca24acb6cba33208d4887b2e3c77R81)

Go version changed to 1.19. 
[(source)](https://github.com/trezor/blockbook/commit/1a476e58f07cb32ffecc41392b339428f7075feb#diff-2e4b6e0eeeea63d6e3c6af432b51d8afc20029970c5849106cc1e0ca899c1701R14)

Rocksdb version changed to 7.5.3. 
[(source)](https://github.com/trezor/blockbook/commit/1a476e58f07cb32ffecc41392b339428f7075feb#diff-2e4b6e0eeeea63d6e3c6af432b51d8afc20029970c5849106cc1e0ca899c1701R15)

