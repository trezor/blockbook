# Tron Sync Tuning

Fast Tron initial sync needs tuning on both sides of the RPC boundary. In the
observed setup, the bottleneck was backend block-hash/block lookup latency, not
Blockbook Go GC or RocksDB writes.

## java-tron

Keep the backend on LevelDB, keep transaction history enabled, enable the LevelDB read tuning and raise the
HTTP connection limit enough for Blockbook's worker count:

```hocon
storage {
  db.engine = "LEVELDB"
  db.sync = false
  transHistory.switch = "on"

  # LevelDB read tuning from Tron TIP-343. Raises fd usage.
  default = {
    maxOpenFiles = 100
  }
  defaultM = {
    maxOpenFiles = 500
  }
  defaultL = {
    maxOpenFiles = 1000
  }
}

node {
  # With Blockbook -workers=32, start around 200.
  maxHttpConnectNumber = 200
}
```

Run java-tron with enough heap, but leave RAM for the OS page cache. On large
hosts, `-Xms32g -Xmx32g` with G1GC is a good starting point. Validate with GC
logs and disk latency; a larger heap is not automatically faster.

Keep event subscription minimal for Blockbook sync:

```hocon
event.subscribe {
  topics = [
    { triggerName = "block", enable = true, topic = "block", solidified = false },
    { triggerName = "solidity", enable = true, topic = "solidity" }
  ]
}
```

Do not enable `transaction`, `contractevent`, or `contractlog` topics just for
Blockbook initial sync.

## Blockbook

Run initial sync with enough workers to keep java-tron busy:

```ini
ExecStart=.../blockbook ... -sync -workers=32 ...
```

Pair this with a sufficiently large Tron HTTP connection pool in Blockbook and
`node.maxHttpConnectNumber` in java-tron. Raising workers without raising the
backend connection limit just moves the queue.

## Verify

Useful indicators while syncing:

* Blockbook log `elapsed` per 1000 blocks
* `blockbook_rpc_latency{method="GetBlockHash"}` and `method="GetBlock"`, increased latency means the bottleneck is on the Java-Tron side
* java-tron GC pauses and heap after GC
* disk utilization and await, for example `iostat -xz 1`
* open file descriptors for both services
* `blockbook_index_block_not_found_retries`

With the tuning above and native hash lookup, observed throughput reached about
7.5-9.5 seconds per 1000 historical blocks on the tested setup.
