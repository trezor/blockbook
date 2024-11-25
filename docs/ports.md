# Registry of ports

| coin             | blockbook public | blockbook internal | backend rpc | backend service ports (zmq) |
|------------------|------------------|--------------------|-------------|-----------------------------|
| Bitcoin          | 9130             | 9030               | 8030        | 38330                       |
| Bitcoin Signet   | 19120            | 19020              | 18020       | 48320                       |
| Bitcoin Regtest  | 19121            | 19021              | 18021       | 48321                       |
| Bitcoin Testnet4 | 19129            | 19029              | 18029       | 48329                       |
| Bitcoin Testnet  | 19130            | 19030              | 18030       | 48330                       |

> NOTE: This document is generated from coin definitions in `configs/coins` using command `go run contrib/scripts/check-and-generate-port-registry.go -w`.
