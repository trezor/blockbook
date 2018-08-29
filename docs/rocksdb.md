# Data storage in RocksDB

Blockbook stores data the key-value store RocksDB. Data are stored in binary form to save space.
The data are separated to different column families:

- **default**

  at the moment not used, will store statistical data etc.

- **height** - maps *block height* to *block hash*

  *Block heigh* stored as array of 4 bytes (big endian uint32)
  *Block hash* stored as array of 32 bytes

  Example - the first four blocks (all data hex encoded)
```
0x00000000 : 0x000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943
0x00000001 : 0x00000000b873e79784647a6c82962c70d228557d24a747ea4d1b8bbe878e1206
0x00000002 : 0x000000006c02c8ea6e4ff69651f7fcde348fb9d557a06e6957b65552002a7820
0x00000003 : 0x000000008b896e272758da5297bcd98fdc6d97c9b765ecec401e286dc1fdbe10
```

- **outputs** -  maps *output script+block height* to *array of outpoints*

  *Output script (ScriptPubKey)+block height* stored as variable length array of bytes for output script + 4 bytes (big endian uint32) block height
  *array of outpoints* stored as array of 32 bytes for transaction id + variable length outpoint index for each outpoint

  Example - (all data hex encoded)
```
0x001400efeb484a24a1c1240eafacef8566e734da429c000e2df6 : 0x1697966cbd76c75eb9fc736dfa3ba0bc045999bab1e8b10082bc0ba546b0178302
0xa9143e3d6abe282d92a28cb791697ba001d733cefdc7870012c4b1 : 0x7246e79f97b5f82e7f51e291d533964028ec90be0634af8a8ef7d5a903c7f6d301
```

- **inputs** - maps *transaction outpoint* to *input transaction* that spends it

  *Transaction outpoint* stored as array of 32 bytes for transaction id + variable length outpoint index
  *Input transaction* stored as array of 32 bytes for transaction id + variable length input index

  Example - (all data hex encoded)
```
0x7246e79f97b5f82e7f51e291d533964028ec90be0634af8a8ef7d5a903c7f6d300 : 0x0a7aa90ea0269c79f844c516805e4cac594adb8830e56fca894b66aab19136a428
0x7246e79f97b5f82e7f51e291d533964028ec90be0634af8a8ef7d5a903c7f6d301 : 0x4303a9fcfe6026b4d33ba488df6443c9a99bca7b7fcb7c6f6cd65cea24a749b700
```

