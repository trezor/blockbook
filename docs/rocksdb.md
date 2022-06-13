# Data storage in RocksDB

**Blockbook** stores data the key-value store [RocksDB](https://github.com/facebook/rocksdb/wiki). As there are multiple indexes, Blockbook uses RocksDB **column families** feature to store indexes separately.

>The database structure is described in golang pseudo types in the form *(name type)*. 
>
>Operators used in the description: 
>- *->* mapping from key to value. 
>- *\+* concatenation, 
>- *[]* array
>
>Types used in the description:
>- *[]byte* - variable length array of bytes
>- *[32]byte* - fixed length array of bytes (32 bytes long in this case)
>- *uint32* - unsigned integer, stored as array of 4 bytes in big endian*
>- *vint*, *vuint* - variable length signed/unsigned int
>- *addrDesc* - address descriptor, abstraction of an address.
For Bitcoin type coins it is the transaction output script, stored as variable length array of bytes. 
For Ethereum type coins it is fixed size array of 20 bytes.
>- *bigInt* - unsigned big integer, stored as length of the array (1 byte) followed by array of bytes of big int, i.e. *(int_len byte)+(int_value []byte)*. Zero is stored as one byte of value 0.

**Database structure:**

The database structure described here is of Blockbook version **0.3.6** (internal data format version 5). 

The database structure for **Bitcoin type** and **Ethereum type** coins is slightly different. Column families used for both types:
- default, height, addresses, transactions, blockTxs

Column families used only by **Bitcoin type** coins:
- addressBalance, txAddresses

Column families used only by **Ethereum type** coins:
- addressContracts

**Column families description:**

- **default**

  Stores internal state in json format, under the key *internalState*. 
  
  Most important internal state values are:
  - coin - which coin is indexed in DB
  - data format version - currently 5
  - dbState - closed, open, inconsistent
    
  Blockbook is checking on startup these values and does not allow to run against wrong coin, data format version and in inconsistent state. The database must be recreated if the internal state does not match.

- **height** 

    Maps *block height* to *block hash* and additional data about block.
    ```
    (height uint32) -> (hash [32]byte)+(time uint32)+(nr_txs vuint)+(size vuint)
    ```

- **addresses**

    Maps *addrDesc+block height* to *array of transactions with array of input/output indexes*.
    
    The *block height* in the key is stored as bitwise complement ^ of the height to sort the keys in the order from newest to oldest.
    
    As there can be multiple inputs/outputs for the same address in one transaction, each txid is followed by variable length array of input/output indexes.
    The index values in the array are multiplied by two, the last element of the array has the lowest bit set to 1.
    Input or output is distinguished by the sign of the index, output is positive, input is negative (by operation bitwise complement ^ performed on the number).   
    ```
    (addrDesc []byte)+(^height uint32) -> []((txid [32]byte)+[](index vint))
    ```

- **addressBalance** (used only by Bitcoin type coins)

    Maps *addrDesc* to *number of transactions*, *sent amount*, *total balance* and a list of *unspent transactions outputs (UTXOs)*, ordered from oldest to newest
    ```
    (addrDesc []byte) -> (nr_txs vuint)+(sent_amount bigInt)+(balance bigInt)+
                         []((txid [32]byte)+(vout vuint)+(block_height vuint)+(amount bigInt))
    ```

- **txAddresses** (used only by Bitcoin type coins)

    Maps *txid* to *block height* and array of *input addrDesc* with *amounts* and array of *output addrDesc* with *amounts*, with flag if output is spent. In case of spent output, *addrDesc_len* is negative (negative sign is achieved by bitwise complement ^).
    ```
    (txid []byte) -> (height vuint)+
                     (nr_inputs vuint)+[]((addrDesc_len vuint)+(addrDesc []byte)+(amount bigInt))+
                     (nr_outputs vuint)+[]((addrDesc_len vint)+(addrDesc []byte)+(amount bigInt))
    ```

- **addressContracts** (used only by Ethereum type coins)

    Maps *addrDesc* to *total number of transactions*, *number of non contract transactions* and array of *contracts* with *number of transfers* of given address.
    ```
    (addrDesc []byte) -> (total_txs vuint)+(non-contract_txs vuint)+[]((contractAddrDesc []byte)+(nr_transfers vuint))
    ```

- **blockTxs**

    Maps *block height* to data necessary for blockchain rollback. Only last 300 (by default) blocks are kept. 
    The content of value data differs for Bitcoin and Ethereum types.

    - Bitcoin type

    The value is an array of *txids* and *input points* in the block.
    ```
    (height uint32) -> []((txid [32]byte)+(nr_inputs vuint)+[]((txid [32]byte)+(index vint)))
    ```

    - Ethereum type
    
    The value is an array of transaction data. For each transaction is stored *txid*,
     *from* and *to* address descriptors and array of *contract address descriptors* with *transfer address descriptors*.
    ```
    (height uint32) -> []((txid [32]byte)+(from addrDesc)+(to addrDesc)+(nr_contracts vuint)+[]((contract addrDesc)+(addr addrDesc)))
    ```

- **transactions**

    Transaction cache, *txdata* is generated by coin specific parser function PackTx.
    ```
    (txid []byte) -> (txdata []byte)
    ```

- **fiatRates**

    Stores fiat rates in json format.
    ```
    (timestamp YYYYMMDDhhmmss) -> (rates json)
    ```


The `txid` field as specified in this documentation is a byte array of fixed size with length 32 bytes (*[32]byte*), however some coins may define other fixed size lengths.
