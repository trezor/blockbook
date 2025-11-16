# Data storage in RocksDB

**Blockbook** stores data the key-value store [RocksDB](https://github.com/facebook/rocksdb/wiki). As there are multiple indexes, Blockbook uses RocksDB **column families** feature to store indexes separately.

> The database structure is described in golang pseudo types in the form _(name type)_.
>
> Operators used in the description:
>
> - _->_ mapping from key to value.
> - _\+_ concatenation,
> - _[]_ array
>
> Types used in the description:
>
> - _[]byte_ - variable length array of bytes
> - _[32]byte_ - fixed length array of bytes (32 bytes long in this case)
> - _uint32_ - unsigned integer, stored as array of 4 bytes in big endian\*
> - _vint_, _vuint_ - variable length signed/unsigned int
> - _addrDesc_ - address descriptor, abstraction of an address.
>   For Bitcoin type coins it is the transaction output script, stored as variable length array of bytes.
>   For Ethereum type coins it is fixed size array of 20 bytes.
> - _bigInt_ - unsigned big integer, stored as length of the array (1 byte) followed by array of bytes of big int, i.e. _(int_len byte)+(int_value []byte)_. Zero is stored as one byte of value 0.
> - _float32_ - float32 number stored as _uint32_
> - string - string stored as `(len vuint)+(value []byte)`

**Database structure:**

The database structure described here is of Blockbook version **0.5.0** (internal data format version 7).

The database structure for **Bitcoin type** and **Ethereum type** coins is different. Column families used for both types:

- default, height, addresses, transactions, blockTxs, fiatRates

Column families used only by **Bitcoin type** coins:

- addressBalance, txAddresses

Column families used only by **Ethereum type** coins:

- addressContracts, internalData, contracts, functionSignatures, blockInternalDataErrors, addressAliases

**Column families description:**

- **default**

  Stores internal state in json format, under the key _internalState_.

  Most important internal state values are:

  - coin - which coin is indexed in DB
  - data format version - currently 6
  - dbState - closed, open, inconsistent

  Blockbook is checking on startup these values and does not allow to run against wrong coin, data format version and in inconsistent state. The database must be recreated if the internal state does not match.

- **height**

  Maps _block height_ to _block hash_ and additional data about block.

  ```
  (height uint32) -> (hash [32]byte)+(time uint32)+(nr_txs vuint)+(size vuint)
  ```

- **addresses**

  Maps _addrDesc+block height_ to _array of transactions with array of input/output indexes_.

  The _block height_ in the key is stored as bitwise complement ^ of the height to sort the keys in the order from newest to oldest.

  As there can be multiple inputs/outputs for the same address in one transaction, each txid is followed by variable length array of input/output indexes.
  The index values in the array are multiplied by two, the last element of the array has the lowest bit set to 1.
  Input or output is distinguished by the sign of the index, output is positive, input is negative (by operation bitwise complement ^ performed on the number).

  ```
  (addrDesc []byte)+(^height uint32) -> []((txid [32]byte)+[](index vint))
  ```

- **addressBalance** (used only by Bitcoin type coins)

  Maps _addrDesc_ to _number of transactions_, _sent amount_, _total balance_ and a list of _unspent transactions outputs (UTXOs)_, ordered from oldest to newest

  ```
  (addrDesc []byte) -> (nr_txs vuint)+(sent_amount bigInt)+(balance bigInt)+
                       []((txid [32]byte)+(vout vuint)+(block_height vuint)+(amount bigInt))
  ```

- **txAddresses** (used only by Bitcoin type coins)

  Maps _txid_ to _block height_ and array of _input addrDesc_ with _amounts_ and array of _output addrDesc_ with _amounts_, with flag if output is spent. In case of spent output, _addrDesc_len_ is negative (negative sign is achieved by bitwise complement ^).

  ```
  (txid []byte) -> (height vuint)+
                   (nr_inputs vuint)+[]((addrDesc_len vuint)+(addrDesc []byte)+(amount bigInt))+
                   (nr_outputs vuint)+[]((addrDesc_len vint)+(addrDesc []byte)+(amount bigInt))
  ```

- **addressContracts** (used only by Ethereum type coins)

  Maps _addrDesc_ to _total number of transactions_, _number of non contract transactions_, _number of internal transactions_
  and array of _contracts_ with _number of transfers_ of given address.

  ```
  (addrDesc []byte) -> (total_txs vuint)+(non-contract_txs vuint)+(internal_txs vuint)+(contracts vuint)+
                       []((contractAddrDesc []byte)+(type+4*nr_transfers vuint))+
                       <(value bigInt) if ERC20> or
                         <(nr_values vuint)+[](id bigInt) if ERC721> or
                         <(nr_values vuint)+[]((id bigInt)+(value bigInt)) if ERC1155>
  ```

- **internalData** (used only by Ethereum type coins)

  Maps _txid_ to _type (CALL 0 | CREATE 1)_, _addrDesc of created contract for CREATE type_, array of _type (CALL 0 | CREATE 1 | SELFDESTRUCT 2)_, _from addrDesc_, _to addrDesc_, _value bigInt_ and possible _error_.

  ```
  (txid []byte) -> (type+2*nr_transfers vuint)+<(addrDesc []byte) if CREATE>+
                   []((type byte)+(fromAddrDesc []byte)+(toAddrDesc []byte)+(value bigInt))+
                   (error []byte)
  ```

- **blockTxs**

  Maps _block height_ to data necessary for blockchain rollback. Only last 300 (by default) blocks are kept.
  The content of value data differs for Bitcoin and Ethereum types.

  - Bitcoin type

  The value is an array of _txids_ and _input points_ in the block.

  ```
  (height uint32) -> []((txid [32]byte)+(nr_inputs vuint)+[]((txid [32]byte)+(index vint)))
  ```

  - Ethereum type

  The value is an array of transaction data. For each transaction is stored _txid_,
  _from_ and _to_ address descriptors and array of contract transfer infos consisting of
  _from_, _to_ and _contract_ address descriptors, _type (ERC20 0 | ERC721 1 | ERC1155 2)_ and value (or list of id+value for ERC1155)

  ```
  (height uint32) -> [](
                        (txid [32]byte)+(from addrDesc)+(to addrDesc)+(nr_contracts vuint)+
                        []((from addrDesc)+(to addrDesc)+(contract addrDesc)+(type byte)+
                        <(value bigInt) if ERC20 or ERC721> or
                          <(nr_values vuint)+[]((id bigInt)+(value bigInt)) if ERC1155>)
                       )
  ```

- **transactions**

  Transaction cache, _txdata_ is generated by coin specific parser function PackTx.

  ```
  (txid []byte) -> (txdata []byte)
  ```

- **fiatRates**

  Stored daily fiat rates, one day as one entry.

  ```
  (timestamp YYYYMMDDhhmmss) -> (nr_currencies vuint)+[]((currency string)+(rate float32))+
                                (nr_tokens vuint)+[]((tokenContract string)+(tokenRate float32))
  ```

- **contracts** (used only by Ethereum type coins)

  Maps contract _addrDesc_ to information about contract - _name_, _symbol_, _type_ (ERC20,ERC721 or ERC1155), _decimals_, _created_ and _destructed_ in block height

  ```
  (addrDesc []byte) -> (name string)+(symbol string)+(type string)+(decimals vuint)+
                       (createdInBlock vuint)+(destroyedInBlock vuint)
  ```

- **functionSignatures** (used only by Ethereum type coins)

  Database of four byte signatures downloaded from https://www.4byte.directory/.

  ```
  (fourBytes uint32)+(id uint32) -> (signatureName string)+[]((parameter string))
  ```

- **blockInternalDataErrors** (used only by Ethereum type coins)

  Errors when fetching internal data from backend. Stored so that the action can be retried.

  ```
  (blockHeight uint32) -> (blockHash [32]byte)+(retryCount byte)+(errorMessage []byte)
  ```

- **addressAliases** (used only by Ethereum type coins)

  Maps _address_ to address ENS name.

  ```
  (address []byte) -> (ensName []byte)
  ```

**Note:**
The `txid` field as specified in this documentation is a byte array of fixed size with length 32 bytes (_[32]byte_), however some coins may define other fixed size lengths.
